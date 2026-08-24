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
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

// Regenerate the golden files after changing a renderer on purpose:
//
//	UPDATE_GOLDEN=1 go test ./tools/changelog/
var updateGolden = os.Getenv("UPDATE_GOLDEN") != ""

const fixtureMilestone = "v1.21.0"

func fixtureSections() Sections {
	return Sections{"core": "", "vm": "", "vd": "", "cli": "", "docs": "", "ci": levelLow}
}

// fixtureEntries is one milestone with every shape a renderer has to handle:
// both published types, the two that are not published, a section that forces
// low impact, a migration note of one line and of several, a summary that a
// plain YAML scalar cannot carry, and two entries of the same section.
func fixtureEntries() []Entry {
	url := func(iid int) string {
		return fmt.Sprintf("https://fox.flant.com/deckhouse/virtualization/virtualization/-/merge_requests/%d", iid)
	}
	return []Entry{
		{
			Section: "vm", Type: "feature", Summary: "Virtual machines can be migrated to a chosen node.",
			MRIID: 101, MRURL: url(101),
		},
		{
			Section: "core", Type: "feature", Summary: "The image registry runs on containerd v2.",
			ImpactLevel: levelHigh, Impact: "Every node restarts once during the update.",
			MRIID: 102, MRURL: url(102),
		},
		{
			Section: "vd", Type: "fix", Summary: "Deleting a disk no longer leaves its volume claim behind.",
			MRIID: 103, MRURL: url(103),
		},
		{
			Section: "vd", Type: "fix", Summary: "A disk of 253 characters can be created: the name is no longer cut.",
			MRIID: 104, MRURL: url(104),
		},
		{
			Section: "core", Type: "chore", Summary: "Fixed vulnerabilities:\n- CVE-2026-46600\n- CVE-2025-27144",
			MRIID: 105, MRURL: url(105),
		},
		{
			Section: "ci", Type: "fix", Summary: "The nightly pipeline runs the suite in parallel.",
			MRIID: 106, MRURL: url(106),
		},
		{
			Section: "docs", Type: "docs", Summary: "The installation page lists the new setting.",
			MRIID: 107, MRURL: url(107),
		},
		{
			Section: "cli", Type: "feature", Summary: "The inventory carries host variables.",
			ImpactLevel: levelHigh, Impact: "Re-generate the inventory:\nd8 v ansible-inventory > hosts.yaml",
			MRIID: 108, MRURL: url(108),
		},
	}
}

func TestGoldenFiles(t *testing.T) {
	entries, sections := fixtureEntries(), fixtureSections()
	minor := MinorVersion(fixtureMilestone)

	block := RenderMilestoneBlock(entries, sections, fixtureMilestone)
	files := map[string]string{
		"CHANGELOG-" + fixtureMilestone + ".yml": RenderYAML(entries, nil),
		"CHANGELOG-" + minor + ".md":             MergeMinorMarkdown("", minor, fixtureMilestone, block),
		"changelog-merge-request.md":             RenderReleaseMarkdown(entries, sections, fixtureMilestone),
	}

	for name, got := range files {
		golden := filepath.Join("testdata", name)
		if updateGolden {
			if err := os.WriteFile(golden, []byte(got), 0o644); err != nil {
				t.Fatal(err)
			}
			continue
		}
		want, err := os.ReadFile(golden)
		if err != nil {
			t.Fatal(err)
		}
		if got != string(want) {
			t.Errorf("%s does not match the golden file:\n--- got ---\n%s", name, got)
		}
	}
}

// TestRenderedYAMLIsReadable: whatever the renderer writes, a YAML reader has
// to get the summaries back unchanged. This is what quoting is for.
func TestRenderedYAMLIsReadable(t *testing.T) {
	entries := append(fixtureEntries(),
		Entry{Section: "vm", Type: "fix", Summary: "fix: a colon and a #hash", MRIID: 201, MRURL: "url"},
		Entry{Section: "vm", Type: "fix", Summary: "- a leading dash", MRIID: 202, MRURL: "url"},
		Entry{Section: "vm", Type: "fix", Summary: "a trailing space ", MRIID: 203, MRURL: "url"},
		Entry{Section: "vm", Type: "fix", Summary: "two\nlines", MRIID: 204, MRURL: "url"},
		Entry{Section: "vm", Type: "fix", Summary: "<html> & \"quotes\"", MRIID: 205, MRURL: "url"},
	)

	var parsed map[string]map[string][]struct {
		Summary     string `yaml:"summary"`
		PullRequest string `yaml:"pull_request"`
		Impact      string `yaml:"impact"`
	}
	rendered := RenderYAML(entries, nil)
	if err := yaml.Unmarshal([]byte(rendered), &parsed); err != nil {
		t.Fatalf("the rendered file is not valid YAML: %v\n%s", err, rendered)
	}

	published := map[string]string{}
	for _, buckets := range parsed {
		for _, items := range buckets {
			for _, item := range items {
				published[item.Summary] = item.Impact
			}
		}
	}
	for _, entry := range entries {
		if _, ok := typeBuckets[entry.Type]; !ok {
			continue
		}
		impact, ok := published[entry.Summary]
		if !ok {
			t.Errorf("summary %q did not survive the round trip", entry.Summary)
			continue
		}
		if impact != entry.Impact {
			t.Errorf("impact of %q: got %q, want %q", entry.Summary, impact, entry.Impact)
		}
	}
}

func TestRenderYAML(t *testing.T) {
	t.Run("a milestone with nothing published is an empty mapping", func(t *testing.T) {
		if got := RenderYAML(nil, nil); got != "{}\n\n" {
			t.Errorf("got %q, want %q", got, "{}\n\n")
		}
		chore := []Entry{{Section: "vm", Type: "chore", Summary: "noise", MRIID: 1}}
		if got := RenderYAML(chore, nil); got != "{}\n\n" {
			t.Errorf("got %q, want the types that are not published to be left out", got)
		}
	})

	t.Run("sections come alphabetically and entries newest first", func(t *testing.T) {
		entries := []Entry{
			{Section: "vm", Type: "fix", Summary: "older", MRIID: 5, MRURL: "url"},
			{Section: "core", Type: "fix", Summary: "unrelated", MRIID: 6, MRURL: "url"},
			{Section: "vm", Type: "fix", Summary: "newer", MRIID: 9, MRURL: "url"},
		}
		got := RenderYAML(entries, nil)
		if strings.Index(got, "core:") > strings.Index(got, "vm:") {
			t.Errorf("sections are not sorted:\n%s", got)
		}
		if strings.Index(got, "newer") > strings.Index(got, "older") {
			t.Errorf("entries are not newest first:\n%s", got)
		}
	})

	t.Run("an entry of a type nobody publishes is reported", func(t *testing.T) {
		var warnings []string
		RenderYAML([]Entry{{Section: "vm", Type: "chore", Summary: "noise", MRIID: 7}},
			func(message string) { warnings = append(warnings, message) })
		if len(warnings) != 1 || !strings.Contains(warnings[0], "!7") {
			t.Errorf("got %v, want one warning naming the merge request", warnings)
		}
	})
}

func TestRenderReleaseMarkdown(t *testing.T) {
	sections := fixtureSections()

	t.Run("what to know before updating comes first", func(t *testing.T) {
		got := RenderReleaseMarkdown(fixtureEntries(), sections, fixtureMilestone)
		know := strings.Index(got, "## Know before update")
		features := strings.Index(got, "## Features")
		if know < 0 || features < 0 || know > features {
			t.Errorf("got:\n%s", got)
		}
		if !strings.Contains(got, " - Every node restarts once during the update.") {
			t.Errorf("the note of the high-impact entry is missing:\n%s", got)
		}
	})

	t.Run("low impact and documentation stay in the files", func(t *testing.T) {
		got := RenderReleaseMarkdown(fixtureEntries(), sections, fixtureMilestone)
		if strings.Contains(got, "runs the suite in parallel") {
			t.Errorf("a low-impact entry reached the description:\n%s", got)
		}
		if strings.Contains(got, "lists the new setting") {
			t.Errorf("a documentation entry reached the description:\n%s", got)
		}
	})

	t.Run("the lines of an entry stay inside its bullet", func(t *testing.T) {
		got := RenderReleaseMarkdown(fixtureEntries(), sections, fixtureMilestone)
		if !strings.Contains(got, "\n   - CVE-2026-46600") {
			t.Errorf("a continuation line left the list:\n%s", got)
		}
		if strings.Contains(got, "\n- CVE-2026-46600") {
			t.Errorf("a continuation line is at column zero:\n%s", got)
		}
	})
}

func TestMergeMinorMarkdown(t *testing.T) {
	sections := fixtureSections()
	entry := func(summary string, iid int) []Entry {
		return []Entry{{Section: "vm", Type: "fix", Summary: summary, MRIID: iid, MRURL: "url"}}
	}

	first := MergeMinorMarkdown("", "v1.21", "v1.21.0",
		RenderMilestoneBlock(entry("older", 1), sections, "v1.21.0"))
	second := MergeMinorMarkdown(first, "v1.21", "v1.21.1",
		RenderMilestoneBlock(entry("newer", 2), sections, "v1.21.1"))

	t.Run("the file keeps every patch of the minor version", func(t *testing.T) {
		for _, want := range []string{"# Changelog v1.21", "## v1.21.0", "## v1.21.1", "older", "newer"} {
			if !strings.Contains(second, want) {
				t.Errorf("%q is missing:\n%s", want, second)
			}
		}
	})

	t.Run("the newest patch is on top", func(t *testing.T) {
		if strings.Index(second, "## v1.21.1") > strings.Index(second, "## v1.21.0") {
			t.Errorf("got:\n%s", second)
		}
	})

	t.Run("generating the same milestone again changes nothing", func(t *testing.T) {
		again := MergeMinorMarkdown(second, "v1.21", "v1.21.1",
			RenderMilestoneBlock(entry("newer", 2), sections, "v1.21.1"))
		if again != second {
			t.Errorf("re-generating is not idempotent:\n--- again ---\n%s\n--- before ---\n%s", again, second)
		}
	})

	t.Run("a milestone with no entries says so", func(t *testing.T) {
		got := RenderMilestoneBlock(nil, sections, "v1.21.2")
		if !strings.Contains(got, "_No changelog entries._") {
			t.Errorf("got:\n%s", got)
		}
	})
}

func TestMinorVersion(t *testing.T) {
	tests := map[string]string{"v1.21.3": "v1.21", "v1.21": "v1.21", "nightly": "nightly"}
	for milestone, want := range tests {
		if got := MinorVersion(milestone); got != want {
			t.Errorf("MinorVersion(%q) = %q, want %q", milestone, got, want)
		}
	}
}

// TestRendersTheReleasedFilesUnchanged renders the changelog of every release
// the repository carries and expects the same bytes back.
//
// It is what keeps this rewrite from reflowing the published changelog: the
// files are the output of the generator being replaced, and a release must not
// show up in the diff of a refactoring. The releases generated before the
// pipeline moved to GitLab are skipped — the action that wrote them folded long
// lines, which the generator this replaces already stopped doing.
func TestRendersTheReleasedFilesUnchanged(t *testing.T) {
	paths, err := filepath.Glob("../../CHANGELOG/CHANGELOG-v*.yml")
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(paths)

	checked := 0
	for _, path := range paths {
		if strings.HasSuffix(path, ".ru.yml") {
			continue
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(raw), ": >-") {
			continue
		}
		entries, err := entriesOfReleasedFile(raw)
		if err != nil {
			t.Errorf("%s: %v", path, err)
			continue
		}
		if got := RenderYAML(entries, nil); got != string(raw) {
			t.Errorf("%s is rendered differently:\n--- got ---\n%s", path, got)
		}
		checked++
	}
	if checked == 0 {
		t.Fatal("no released changelog was checked")
	}
	t.Logf("rendered %d released changelogs unchanged", checked)
}

// entriesOfReleasedFile reads a published changelog back into the entries that
// produced it. The order of the file is the order the renderer has to produce,
// so the merge request numbers are taken from the links and the file is
// expected to be sorted by them already.
func entriesOfReleasedFile(raw []byte) ([]Entry, error) {
	var parsed map[string]map[string][]struct {
		Summary     string `yaml:"summary"`
		PullRequest string `yaml:"pull_request"`
		Impact      string `yaml:"impact"`
	}
	if err := yaml.Unmarshal(raw, &parsed); err != nil {
		return nil, err
	}

	var entries []Entry
	for _, section := range sortedKeys(parsed) {
		for bucket, changeType := range map[string]string{"features": "feature", "fixes": "fix"} {
			for _, item := range parsed[section][bucket] {
				iid := 0
				if index := strings.LastIndex(item.PullRequest, "/"); index >= 0 {
					iid, _ = strconv.Atoi(item.PullRequest[index+1:])
				}
				entries = append(entries, Entry{
					Section: section, Type: changeType, Summary: item.Summary,
					Impact: item.Impact, MRIID: iid, MRURL: item.PullRequest,
				})
			}
		}
	}
	return entries, nil
}
