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
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

// The ADR asks for a few statements about the release as a whole; more than that
// is a detailed section, not an annotation.
const highlightsMax = 5

var (
	versionRe = regexp.MustCompile(`^v\d+\.\d+\.\d+$`)
	domainRe  = regexp.MustCompile(`\]\(https?://`)
	// CVE-2026-1234, GHSA-xxxx-xxxx-xxxx, GO-2026-1234. The exact shapes, or words
	// like "GO-based" start counting as vulnerabilities.
	advisoryRe = regexp.MustCompile(`\b(?:CVE-\d{4}-\d+|GO-\d{4}-\d+|GHSA(?:-[0-9a-z]{4}){3})\b`)
)

// check validates the source. Everything the format cannot make impossible on
// its own is here; ru and en cannot drift apart by construction, so there is
// nothing to compare.
//
// changelogDir holds the generated per-release changelogs; it is empty when they
// are not available, and then the vulnerability check is skipped.
func check(releases []Release, changelogDir string) []string {
	var problems []string
	add := func(format string, args ...any) {
		problems = append(problems, fmt.Sprintf(format, args...))
	}

	seen := map[string]bool{}
	previous := []int(nil)
	for i := range releases {
		release := &releases[i]
		where := release.Version
		if where == "" {
			add("release %d: no version", i+1)
			where = fmt.Sprintf("release %d", i+1)
		} else if !versionRe.MatchString(where) {
			add("%s: not a version, expected vX.Y.Z", where)
		}
		if release.Version != "" {
			if seen[release.Version] {
				add("%s: the version appears twice", where)
			}
			seen[release.Version] = true
		}
		current := versionKey(release.Version)
		if previous != nil && current != nil && !less(current, previous) {
			add("%s: releases go from the newest to the oldest", where)
		}
		if current != nil {
			previous = current
		}

		if _, err := release.date(); err != nil {
			add("%s: %v", where, err)
		}
		if changelogDir != "" {
			problems = append(problems, checkAdvisories(release, changelogDir)...)
		}

		if release.Highlights.empty() {
			add("%s: no highlights, write 1-5 statements about the release", where)
		} else if len(release.Highlights.Groups) > 0 {
			add("%s: highlights are not grouped, they annotate the whole release", where)
		} else if count := len(release.Highlights.Items); count > highlightsMax {
			add("%s: %d highlights, keep 1-5 statements about the release as a whole",
				where, count)
		}

		for index, section := range release.bySection() {
			if section == nil {
				continue
			}
			key := sections[index].Key
			if section.empty() {
				add("%s/%s: an empty section, remove it", where, key)
				continue
			}
			known, unknown := section.groupsInOrder()
			if len(unknown) > 0 {
				add("%s/%s: unknown groups %v, expected one of %v", where, key,
					unknown, groupKeys())
			}
			for _, group := range groups {
				if items, ok := section.Groups[group.Key]; ok && len(items) == 0 {
					add("%s/%s/%s: an empty group, remove it", where, key, group.Key)
				}
			}
			// Item by item in the order they are published, so the same source always
			// reports the same problems in the same order.
			for _, item := range section.Items {
				problems = append(problems, checkItem(where+"/"+key, item)...)
			}
			for _, group := range append(known, unknown...) {
				for _, item := range section.Groups[group] {
					problems = append(problems, checkItem(where+"/"+key+"/"+group, item)...)
				}
			}
		}
	}
	return problems
}

// checkAdvisories requires every vulnerability the changelog of a release lists to
// be named in its notes. A fix released on two lines is announced on both: whoever
// runs 1.7 does not read the notes of 1.6, and a release that carries nothing but
// vulnerability fixes has to say so.
func checkAdvisories(release *Release, changelogDir string) []string {
	path := filepath.Join(changelogDir, "CHANGELOG-"+release.Version+".yml")
	changelog, err := os.ReadFile(path)
	if err != nil {
		return nil // a release without a generated changelog: nothing to compare with
	}

	named := map[string]bool{}
	for _, section := range release.bySection() {
		for _, item := range section.allItems() {
			for _, id := range advisoryRe.FindAllString(item.En+" "+item.Ru, -1) {
				named[id] = true
			}
		}
	}

	var missing []string
	seen := map[string]bool{}
	for _, id := range advisoryRe.FindAllString(string(changelog), -1) {
		if !named[id] && !seen[id] {
			seen[id] = true
			missing = append(missing, id)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	sort.Strings(missing)
	return []string{fmt.Sprintf("%s: %s lists %s, and the notes do not name them; a "+
		"vulnerability fix is announced in every release that carries it",
		release.Version, filepath.Base(path), strings.Join(missing, ", "))}
}

func checkItem(where string, item Item) []string {
	var problems []string
	report := func(problem, text string) {
		problems = append(problems, fmt.Sprintf("%s: %s:\n      %s", where, problem, cut(text)))
	}

	if strings.TrimSpace(item.En) == "" {
		problems = append(problems, fmt.Sprintf("%s: an item has no en text", where))
	} else if hasCyrillic(item.En) {
		report("Cyrillic in the en text, it reaches the Console as is", item.En)
	}
	if strings.TrimSpace(item.Ru) == "" {
		report("an item has no ru text", item.En)
	} else if !hasCyrillic(item.Ru) {
		report("an item is not translated", item.Ru)
	}
	for _, text := range []string{item.En, item.Ru} {
		if domainRe.MatchString(text) {
			report("a link with a domain, release notes link to documentation by path", text)
		}
		// A note is rendered as a list item, so the lines after the first one have to
		// be indented to stay inside it; flush left they only hold together by the
		// laziness of the markdown parser.
		lines := strings.Split(text, "\n")
		for _, line := range lines[1:] {
			if strings.TrimSpace(line) != "" && !strings.HasPrefix(line, "  ") {
				report("a continued line of a note is not indented by two spaces", line)
			}
		}
	}
	return problems
}

func hasCyrillic(text string) bool {
	for _, symbol := range text {
		if unicode.Is(unicode.Cyrillic, symbol) {
			return true
		}
	}
	return false
}

func cut(text string) string {
	first, _, _ := strings.Cut(text, "\n")
	if symbols := []rune(first); len(symbols) > 110 {
		return string(symbols[:110])
	}
	return first
}

func versionKey(version string) []int {
	parts := strings.Split(strings.TrimPrefix(version, "v"), ".")
	key := make([]int, 0, len(parts))
	for _, part := range parts {
		number, err := strconv.Atoi(part)
		if err != nil {
			return nil
		}
		key = append(key, number)
	}
	return key
}

func less(left, right []int) bool {
	for i := range left {
		if i >= len(right) || left[i] != right[i] {
			return i < len(right) && left[i] < right[i]
		}
	}
	return len(left) < len(right)
}

func groupKeys() []string {
	keys := make([]string, 0, len(groups))
	for _, group := range groups {
		keys = append(keys, group.Key)
	}
	return keys
}
