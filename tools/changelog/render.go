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
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// The files are written by hand rather than by a YAML or Markdown printer on
// purpose. Their shape is the one deckhouse/changelog-action@v2.6.0 produced
// for years — quoting, order, blank lines and all — and every CHANGELOG file in
// the repository already has it. A printer would reflow all of them on the
// first run and bury the actual change of a release in the diff. Reading is
// where a real parser is needed, and that is where it is used.

// RenderYAML writes CHANGELOG-<milestone>.yml.
//
//	<section>:
//	  features:
//	    - summary: <text>
//	      pull_request: <merge request url>
//	      impact: <migration note, when the entry carries one>
//	  fixes:
//	    ...
//
// Sections come alphabetically and the entries of each newest first. Only
// feature and fix are published; a milestone with none of them is written as an
// empty mapping, the way the generator this replaces wrote it.
func RenderYAML(entries []Entry, warn func(string)) string {
	type buckets struct {
		features []Entry
		fixes    []Entry
	}
	grouped := map[string]*buckets{}
	for _, entry := range entries {
		bucket, published := typeBuckets[entry.Type]
		if !published {
			if warn != nil {
				warn(fmt.Sprintf("!%d has type '%s', which is not published (only feature and fix are), skipping",
					entry.MRIID, entry.Type))
			}
			continue
		}
		if grouped[entry.Section] == nil {
			grouped[entry.Section] = &buckets{}
		}
		if bucket == "features" {
			grouped[entry.Section].features = append(grouped[entry.Section].features, entry)
		} else {
			grouped[entry.Section].fixes = append(grouped[entry.Section].fixes, entry)
		}
	}

	if len(grouped) == 0 {
		return "{}\n\n"
	}

	var lines []string
	for _, section := range sortedKeys(grouped) {
		lines = append(lines, section+":")
		for _, bucket := range []struct {
			name    string
			entries []Entry
		}{
			{"features", grouped[section].features},
			{"fixes", grouped[section].fixes},
		} {
			if len(bucket.entries) == 0 {
				continue
			}
			items := append([]Entry(nil), bucket.entries...)
			sort.SliceStable(items, func(i, j int) bool { return items[i].MRIID > items[j].MRIID })
			lines = append(lines, "  "+bucket.name+":")
			for _, entry := range items {
				lines = append(lines,
					"    - summary: "+yamlScalar(entry.Summary),
					"      pull_request: "+entry.MRURL,
				)
				lines = append(lines, impactLines(entry.Impact)...)
			}
		}
	}
	return strings.Join(lines, "\n") + "\n\n"
}

// impactLines writes the migration note of a high-impact entry, which upstream
// puts right after the merge request link. A note of several lines becomes a
// literal block so that its line breaks survive.
func impactLines(impact string) []string {
	switch {
	case impact == "":
		return nil
	case !strings.Contains(impact, "\n"):
		return []string{"      impact: " + yamlScalar(impact)}
	}
	lines := []string{"      impact: |-"}
	for _, line := range strings.Split(impact, "\n") {
		if line == "" {
			lines = append(lines, "")
			continue
		}
		lines = append(lines, "        "+line)
	}
	return lines
}

// yamlScalar writes a value the plain way when that is safe, and double-quoted
// when a plain scalar would mean something else to a YAML reader.
func yamlScalar(value string) string {
	if value == "" {
		return `""`
	}
	if strings.ContainsAny(value, ":#") ||
		strings.ContainsAny(value[:1], "-?,[]{}'\"&*!|>%@`") ||
		strings.HasSuffix(value, " ") ||
		strings.Contains(value, "\n") {
		return quote(value)
	}
	return value
}

// quote writes a JSON string, which is also a valid double-quoted YAML scalar.
// The Go encoder escapes <, > and & for HTML by default; nothing here is HTML,
// and the escapes would show up in the published changelog.
func quote(value string) string {
	var out bytes.Buffer
	encoder := json.NewEncoder(&out)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(value); err != nil {
		// A string always encodes.
		return strconv.Quote(value)
	}
	return strings.TrimRight(out.String(), "\n")
}

// RenderReleaseMarkdown writes the description of the changelog merge request,
// which is also the text of the GitLab release: what to know before updating
// first, then the changes themselves.
//
// Low-impact entries and documentation entries stay out of it. They are in the
// files; repeating them here would bury the entries that matter to whoever is
// deciding to update.
func RenderReleaseMarkdown(entries []Entry, sections Sections, milestone string) string {
	var notes []string
	for _, entry := range entries {
		if sections.Level(entry) == levelHigh && entry.Impact != "" {
			notes = append(notes, entry.Impact)
		}
	}
	sort.Strings(notes)

	groups := []struct {
		heading string
		items   []string
	}{{"Know before update", notes}}

	for _, group := range []struct{ heading, changeType string }{
		{"Features", "feature"},
		{"Fixes", "fix"},
		{"Chore", "chore"},
	} {
		var matching []Entry
		for _, entry := range entries {
			if entry.Type == group.changeType && sections.Level(entry) != levelLow {
				matching = append(matching, entry)
			}
		}
		sort.SliceStable(matching, func(i, j int) bool { return matching[i].Section < matching[j].Section })

		items := make([]string, 0, len(matching))
		for _, entry := range matching {
			line := fmt.Sprintf("**[%s]** %s [!%d](%s)", entry.Section, entry.Summary, entry.MRIID, entry.MRURL)
			if entry.Impact != "" {
				line += "\n" + entry.Impact
			}
			items = append(items, line)
		}
		groups = append(groups, struct {
			heading string
			items   []string
		}{group.heading, items})
	}

	lines := []string{"# Changelog " + milestone}
	for _, group := range groups {
		if len(group.items) == 0 {
			continue
		}
		lines = append(lines, "\n## "+group.heading+"\n")
		for _, item := range group.items {
			// The continuation lines of an item — a summary or a note of
			// several lines — are indented to the text column of the bullet.
			// Left at column zero they leave the list and render as separate
			// paragraphs.
			lines = append(lines, " - "+strings.ReplaceAll(item, "\n", "\n   "))
		}
	}
	return strings.Join(lines, "\n") + "\n"
}

// RenderMilestoneBlock writes the part of CHANGELOG-<minor>.md that belongs to
// one patch release. The `## <milestone>` heading is what lets MergeMinorMarkdown
// recognise the block later and replace it instead of adding a second one.
func RenderMilestoneBlock(entries []Entry, sections Sections, milestone string) string {
	grouped := map[string][]Entry{}
	for _, entry := range entries {
		grouped[entry.Section] = append(grouped[entry.Section], entry)
	}

	lines := []string{"## " + milestone, ""}
	if len(grouped) == 0 {
		lines = append(lines, "_No changelog entries._")
		return strings.TrimRight(strings.Join(lines, "\n"), "\n \t") + "\n"
	}
	for _, section := range sortedKeys(grouped) {
		lines = append(lines, "### "+section, "")
		for _, entry := range grouped[section] {
			lines = append(lines, fmt.Sprintf("- **%s** (%s): %s ([!%d](%s))",
				entry.Type, sections.Level(entry), entry.Summary, entry.MRIID, entry.MRURL))
		}
		lines = append(lines, "")
	}
	return strings.TrimRight(strings.Join(lines, "\n"), "\n \t") + "\n"
}

// ParseMinorBlocks splits CHANGELOG-<minor>.md back into the block of every
// patch release it holds.
//
// The file is this tool's own output, so its headings are known to sit at the
// start of a line and to be the only ones of their level; splitting on them
// needs no Markdown parser. Whatever comes before the first heading is the
// file header, which is written anew every time.
func ParseMinorBlocks(text string) map[string]string {
	blocks := map[string]string{}
	title := ""
	var current []string
	flush := func() {
		if title != "" {
			blocks[title] = strings.TrimRight(strings.Join(current, "\n"), "\n \t") + "\n"
		}
	}
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(line, "## ") {
			flush()
			title = strings.TrimSpace(line[3:])
			current = []string{line}
			continue
		}
		if title != "" {
			current = append(current, line)
		}
	}
	flush()
	return blocks
}

// MergeMinorMarkdown puts the block of one patch release into the cumulative
// file of its minor version, replacing the block that is already there.
//
// The file keeps every patch of the minor version, newest first: rendering only
// the milestone being generated would drop the releases before it.
func MergeMinorMarkdown(existing, minorVersion, milestone, block string) string {
	blocks := ParseMinorBlocks(existing)
	blocks[milestone] = block

	titles := make([]string, 0, len(blocks))
	for title := range blocks {
		titles = append(titles, title)
	}
	sort.SliceStable(titles, func(i, j int) bool {
		return compareVersions(titles[i], titles[j]) > 0
	})

	out := []string{"# Changelog " + minorVersion, ""}
	for _, title := range titles {
		out = append(out, strings.TrimRight(blocks[title], "\n \t"), "")
	}
	return strings.TrimRight(strings.Join(out, "\n"), "\n \t") + "\n"
}

var versionRE = regexp.MustCompile(`^v?(\d+)\.(\d+)(?:\.(\d+))?`)

// compareVersions orders `vX.Y.Z` headings. A heading that is not a version
// sorts as v0.0.0, which puts it last, where an unexpected heading is least in
// the way.
func compareVersions(a, b string) int {
	left, right := versionParts(a), versionParts(b)
	for i := range left {
		if left[i] != right[i] {
			if left[i] > right[i] {
				return 1
			}
			return -1
		}
	}
	return 0
}

func versionParts(title string) [3]int {
	var parts [3]int
	match := versionRE.FindStringSubmatch(title)
	if match == nil {
		return parts
	}
	for i := 0; i < 3; i++ {
		parts[i], _ = strconv.Atoi(match[i+1])
	}
	return parts
}

var minorRE = regexp.MustCompile(`^v(\d+\.\d+)(?:\.\d+)?$`)

// MinorVersion is the minor version a milestone belongs to: v1.21.3 -> v1.21.
// A milestone that is not a version is its own file.
func MinorVersion(milestone string) string {
	match := minorRE.FindStringSubmatch(milestone)
	if match == nil {
		return milestone
	}
	return "v" + match[1]
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
