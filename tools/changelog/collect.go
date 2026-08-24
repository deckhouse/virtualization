/*
Copyright 2026 Flant JSC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

     http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// changelogLabel marks the merge requests this tool opens itself. Their
// descriptions are changelogs, not changelog entries, and reading them back
// would fold a release into the next one.
const changelogLabel = "changelog"

// collectResult is what one milestone produced.
type collectResult struct {
	entries []Entry
	// unread names the blocks YAML could not read, one line each.
	unread []string
}

// runCollect re-generates the changelog files of every milestone it is asked
// for, and opens the merge request that carries them.
func runCollect(sections Sections) error {
	apiURL, err := requireEnv("CI_API_V4_URL")
	if err != nil {
		return err
	}
	projectID, err := requireEnv("CI_PROJECT_ID")
	if err != nil {
		return err
	}
	token, err := requireEnv("GITLAB_API_TOKEN")
	if err != nil {
		return err
	}
	projectPath, err := requireEnv("CI_PROJECT_PATH")
	if err != nil {
		return err
	}
	serverHost, err := requireEnv("CI_SERVER_HOST")
	if err != nil {
		return err
	}
	projectDir, err := requireEnv("CI_PROJECT_DIR")
	if err != nil {
		return err
	}

	baseBranch := envOr("CHANGELOG_BASE_BRANCH", "main")
	openMR := strings.EqualFold(os.Getenv("OPEN_CHANGELOG_MR"), "true")
	gitlab := NewGitLab(apiURL, token, projectID)

	milestones, err := targetMilestones(gitlab, os.Getenv("MILESTONE_TITLE"))
	if err != nil {
		return err
	}
	if len(milestones) == 0 {
		logf("no milestones to process")
		return nil
	}

	// Every changelog branch is cut from the commit the pipeline checked out,
	// so a milestone never carries the commit of the milestone generated before
	// it in the same run.
	baseSHA, err := git(projectDir, "rev-parse", "HEAD")
	if err != nil {
		return err
	}

	var failures []string
	for _, milestone := range milestones {
		logf("milestone %s (iid=%d)", milestone.Title, milestone.IID)
		if err := collectMilestone(collectRequest{
			gitlab:      gitlab,
			sections:    sections,
			milestone:   milestone,
			projectDir:  projectDir,
			projectPath: projectPath,
			serverHost:  serverHost,
			token:       token,
			baseBranch:  baseBranch,
			baseSHA:     baseSHA,
			openMR:      openMR,
		}); err != nil {
			logf("ERROR: milestone %s: %v", milestone.Title, err)
			failures = append(failures, milestone.Title)
		}
	}
	if len(failures) > 0 {
		return fmt.Errorf("failed on milestone(s): %s", strings.Join(failures, ", "))
	}
	return nil
}

type collectRequest struct {
	gitlab      *GitLab
	sections    Sections
	milestone   Milestone
	projectDir  string
	projectPath string
	serverHost  string
	token       string
	baseBranch  string
	baseSHA     string
	openMR      bool
}

func collectMilestone(request collectRequest) error {
	result, err := collectEntries(request.gitlab, request.sections, request.milestone.Title)
	if err != nil {
		return err
	}

	ymlPath, markdownPath, err := writeFiles(request.projectDir, request.sections, request.milestone.Title, result)
	if err != nil {
		return err
	}

	// A milestone with no entries — one just opened, say — gets its files but
	// no merge request: an empty changelog merge request per open milestone is
	// noise. The run after the first entry lands opens it.
	if !request.openMR || len(result.entries) == 0 {
		return nil
	}

	minor := MinorVersion(request.milestone.Title)
	projectURL := fmt.Sprintf("https://%s/%s", request.serverHost, request.projectPath)
	description := RenderReleaseMarkdown(result.entries, request.sections, request.milestone.Title) +
		fmt.Sprintf("\nFor more information, see the [changelog](%s/-/blob/%s/CHANGELOG/CHANGELOG-%s.md) "+
			"and minor version [release changes](%s/-/releases/%s.0).\n",
			projectURL, request.baseBranch, minor, projectURL, minor) +
		unreadSection(result.unread)

	return pushChangelogMR(request, []string{ymlPath, markdownPath}, description)
}

// unreadSection puts the blocks nobody could read in front of whoever reviews
// the changelog merge request. A block that fails to parse costs its author
// their entry, and a warning in the log of a scheduled job is not where that
// gets noticed. Nothing is added when everything was read, so a normal release
// description is unchanged.
func unreadSection(unread []string) string {
	if len(unread) == 0 {
		return ""
	}
	lines := []string{"", "## Entries that could not be read", ""}
	for _, line := range unread {
		lines = append(lines, " - "+line)
	}
	lines = append(lines,
		"",
		"These merge requests carry a ```changes block that is not valid YAML, "+
			"so their entries are missing above. Fix the block in the merge request "+
			"description and re-run the changelog job.",
		"")
	return strings.Join(lines, "\n")
}

// collectEntries reads the entries of every merge request merged into a
// milestone.
func collectEntries(gitlab *GitLab, sections Sections, milestone string) (collectResult, error) {
	var result collectResult

	logf("fetching merged merge requests of %s", milestone)
	mrs, err := gitlab.MergedMergeRequests(milestone)
	if err != nil {
		return result, err
	}
	logf("found %d merged merge request(s) in %s", len(mrs), milestone)

	for _, mr := range mrs {
		if mr.Labels.Has(changelogLabel) {
			logf("skipping changelog merge request !%d", mr.IID)
			continue
		}
		for _, block := range ParseDescription(mr.Description) {
			if block.Err != nil {
				line := fmt.Sprintf("!%d (%s): block #%d: %v", mr.IID, mr.WebURL, block.Index, block.Err)
				logf("WARN: %s", line)
				result.unread = append(result.unread, line)
				continue
			}
			for _, entry := range block.Entries {
				// An entry that names a section nobody publishes is dropped,
				// not refused: the list of sections changes between releases,
				// and a milestone must still generate when an old merge request
				// names a section that has since been retired.
				if _, known := sections[entry.Section]; !known {
					logf("WARN: !%d names section '%s', which is not in %s, skipping the entry",
						mr.IID, entry.Section, defaultSectionsFile)
					continue
				}
				entry.MRIID = mr.IID
				entry.MRURL = mr.WebURL
				result.entries = append(result.entries, entry)
			}
		}
	}
	return result, nil
}

// writeFiles writes the changelog of the milestone and merges it into the
// cumulative file of its minor version.
func writeFiles(projectDir string, sections Sections, milestone string, result collectResult) (string, string, error) {
	directory := filepath.Join(projectDir, "CHANGELOG")
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return "", "", err
	}

	ymlPath := filepath.Join(directory, "CHANGELOG-"+milestone+".yml")
	if err := os.WriteFile(ymlPath, []byte(RenderYAML(result.entries, func(message string) {
		logf("WARN: %s", message)
	})), 0o644); err != nil {
		return "", "", err
	}

	minor := MinorVersion(milestone)
	markdownPath := filepath.Join(directory, "CHANGELOG-"+minor+".md")
	existing, err := os.ReadFile(markdownPath)
	if err != nil && !os.IsNotExist(err) {
		return "", "", err
	}
	block := RenderMilestoneBlock(result.entries, sections, milestone)
	merged := MergeMinorMarkdown(string(existing), minor, milestone, block)
	if err := os.WriteFile(markdownPath, []byte(merged), 0o644); err != nil {
		return "", "", err
	}

	logf("wrote CHANGELOG/%s and CHANGELOG/%s", filepath.Base(ymlPath), filepath.Base(markdownPath))
	return ymlPath, markdownPath, nil
}

// pushChangelogMR commits the files, pushes the branch of the milestone and
// opens the merge request, or refreshes the one that is already open.
func pushChangelogMR(request collectRequest, files []string, description string) error {
	branch := "changelog/" + request.milestone.Title
	directory := request.projectDir

	if _, err := git(directory, "config", "user.email", "ci-changelog@flant.com"); err != nil {
		return err
	}
	if _, err := git(directory, "config", "user.name", "GitLab CI Changelog Bot"); err != nil {
		return err
	}
	if _, err := git(directory, "checkout", "-B", branch, request.baseSHA); err != nil {
		return err
	}
	// Only the files of this milestone: `git add CHANGELOG` would sweep in the
	// files written for the other milestones of the same run.
	if _, err := git(directory, append([]string{"add"}, files...)...); err != nil {
		return err
	}
	if _, err := git(directory, "diff", "--cached", "--quiet"); err == nil {
		logf("no changes for %s; nothing to commit", request.milestone.Title)
		return nil
	}
	// The push rule of the project rejects a commit without a Signed-off-by
	// trailer; the committer set above signs it.
	if _, err := git(directory, "commit", "--signoff", "-m", "Re-generate changelog "+request.milestone.Title); err != nil {
		return err
	}

	// The token goes into the push URL and not into the remote of the working
	// copy: the shell runner keeps that copy between jobs, and a remote with a
	// token in it would outlive this one.
	pushURL := fmt.Sprintf("https://oauth2:%s@%s/%s.git", request.token, request.serverHost, request.projectPath)
	if _, err := git(directory, "push", "--force", pushURL, "HEAD:refs/heads/"+branch); err != nil {
		return fmt.Errorf("pushing %s failed", branch)
	}
	logf("pushed branch %s", branch)

	// The force-push above already refreshed a merge request opened by an
	// earlier run; only its description can be out of date, because entries
	// merged after it was opened would never show up in the body otherwise.
	open, err := request.gitlab.OpenMergeRequests(branch, request.baseBranch)
	if err != nil {
		return err
	}
	if len(open) > 0 {
		if err := request.gitlab.UpdateMergeRequestDescription(open[0].IID, description); err != nil {
			return err
		}
		logf("refreshed the open changelog merge request !%d", open[0].IID)
		return nil
	}

	created, err := request.gitlab.CreateMergeRequest(map[string]any{
		"source_branch":        branch,
		"target_branch":        request.baseBranch,
		"title":                "Changelog " + request.milestone.Title,
		"description":          description,
		"labels":               "changelog,auto,status/backport",
		"milestone_id":         request.milestone.ID,
		"remove_source_branch": true,
	})
	if err != nil {
		return err
	}
	logf("opened the changelog merge request !%d", created.IID)
	return nil
}

// targetMilestones is the milestone that was asked for, or every open one.
func targetMilestones(gitlab *GitLab, title string) ([]Milestone, error) {
	active, err := gitlab.ActiveMilestones()
	if err != nil {
		return nil, err
	}
	title = strings.TrimSpace(title)
	if title == "" {
		logf("no MILESTONE_TITLE set — generating every active milestone")
		return active, nil
	}
	for _, milestone := range active {
		if milestone.Title == title {
			return []Milestone{milestone}, nil
		}
	}
	return nil, fmt.Errorf("milestone '%s' is not among the active ones", title)
}

func git(directory string, args ...string) (string, error) {
	command := exec.Command("git", args...)
	command.Dir = directory
	output, err := command.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %s: %w: %s", args[0], err, strings.TrimSpace(string(output)))
	}
	return strings.TrimSpace(string(output)), nil
}

func envOr(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
