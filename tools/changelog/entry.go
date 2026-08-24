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
	"sort"
	"strings"
)

// defaultSectionsFile lists the sections a changelog entry may name, and the
// impact level some of them force.
const defaultSectionsFile = ".gitlab/ci/changelog-sections.txt"

const (
	levelDefault = "default"
	levelLow     = "low"
	levelHigh    = "high"
)

// knownLevels are the impact levels deckhouse/changelog-action@v2.6.0 knows. An
// entry that names none of them is a "default" one.
var knownLevels = map[string]bool{levelDefault: true, levelLow: true, levelHigh: true}

// allowedTypes are the types a block may carry. Only feature and fix reach the
// published CHANGELOG-*.yml (see typeBuckets); chore and docs are accepted so
// that a change worth recording internally does not have to lie about itself,
// and they end up in the per-minor CHANGELOG-<minor>.md alone.
var allowedTypes = []string{"chore", "docs", "feature", "fix"}

// typeBuckets maps a type to the key it is published under.
var typeBuckets = map[string]string{"feature": "features", "fix": "fixes"}

// Sections maps each allowed section to the impact level it forces, empty when
// it forces none.
type Sections map[string]string

// LoadSections reads the list of allowed sections.
//
// The list uses the upstream `section:forced_impact_level` format: a section
// written as `ci:low` silently holds every entry of that section to low impact,
// however the entry describes itself.
func LoadSections(path string) (Sections, error) {
	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("allowed sections: %w", err)
	}
	sections := Sections{}
	for _, raw := range strings.Split(string(content), "\n") {
		line := strings.TrimSpace(raw)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		name, forced, _ := strings.Cut(line, ":")
		if !knownLevels[forced] {
			forced = ""
		}
		sections[name] = forced
	}
	return sections, nil
}

// Level is the impact level of the entry: the one its section forces, or the
// one the entry names, or "default".
func (s Sections) Level(entry Entry) string {
	if forced := s[entry.Section]; forced != "" {
		return forced
	}
	if entry.ImpactLevel == "" {
		return levelDefault
	}
	return entry.ImpactLevel
}

// Validate reports everything wrong with the entry, worst case being that it
// names a section nobody publishes. It mirrors ChangeEntry.validate() of
// deckhouse/changelog-action@v2.6.0 plus the allowed-sections check.
func (e Entry) Validate(sections Sections) []string {
	level := sections.Level(e)

	var problems []string
	if e.Summary == "" {
		problems = append(problems, "missing summary")
	}
	if !knownLevels[level] {
		problems = append(problems, fmt.Sprintf("invalid impact level '%s'", level))
	}
	if level == levelHigh && e.Impact == "" {
		problems = append(problems, "missing high impact detail (add an 'impact' key)")
	}
	if e.Section == "" {
		problems = append(problems, "missing section")
	}
	if _, ok := indexOf(allowedTypes, e.Type); !ok {
		if e.Type == "" {
			problems = append(problems, "missing type")
		} else {
			problems = append(problems, fmt.Sprintf("invalid type '%s' (allowed: %s)",
				e.Type, strings.Join(allowedTypes, ", ")))
		}
	}
	sort.Strings(problems)

	// Reported last, and only when the section is written: an entry with no
	// section is already accounted for above.
	if _, known := sections[e.Section]; !known && e.Section != "" {
		problems = append(problems, fmt.Sprintf("unknown section '%s' (see %s)",
			e.Section, defaultSectionsFile))
	}
	return problems
}

func indexOf(values []string, value string) (int, bool) {
	for i, candidate := range values {
		if candidate == value {
			return i, true
		}
	}
	return 0, false
}
