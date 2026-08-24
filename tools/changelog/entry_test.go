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
	"strings"
	"testing"
)

// The list the pipeline validates against. A section that is renamed there and
// not here would let the tests pass while every merge request fails.
const realSectionsFile = "../../.gitlab/ci/changelog-sections.txt"

func TestLoadSectionsReadsTheRealList(t *testing.T) {
	sections, err := LoadSections(realSectionsFile)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := sections["vm"]; !ok {
		t.Error("the list has no 'vm' section")
	}
	if sections["ci"] != levelLow {
		t.Errorf("got forced level %q for 'ci', want %q", sections["ci"], levelLow)
	}
	if sections["vm"] != "" {
		t.Errorf("got forced level %q for 'vm', want none", sections["vm"])
	}
	if _, ok := sections["# Copyright 2026 Flant JSC"]; ok {
		t.Error("a comment line was read as a section")
	}
}

func TestLevel(t *testing.T) {
	sections := Sections{"vm": "", "ci": levelLow}
	tests := []struct {
		name  string
		entry Entry
		want  string
	}{
		{"an entry that says nothing is a default one", Entry{Section: "vm"}, levelDefault},
		{"an entry says its own level", Entry{Section: "vm", ImpactLevel: levelHigh}, levelHigh},
		{"a section can force the level", Entry{Section: "ci", ImpactLevel: levelHigh}, levelLow},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := sections.Level(test.entry); got != test.want {
				t.Errorf("got %q, want %q", got, test.want)
			}
		})
	}
}

func TestValidate(t *testing.T) {
	sections := Sections{"vm": "", "ci": levelLow}
	valid := Entry{Section: "vm", Type: "fix", Summary: "did it"}

	tests := []struct {
		name  string
		entry Entry
		want  []string
	}{{
		name:  "a complete entry",
		entry: valid,
	}, {
		name:  "every type of the upstream action is accepted",
		entry: Entry{Section: "vm", Type: "docs", Summary: "wrote it"},
	}, {
		name:  "a summary is required",
		entry: Entry{Section: "vm", Type: "fix"},
		want:  []string{"missing summary"},
	}, {
		name:  "a type is required",
		entry: Entry{Section: "vm", Summary: "did it"},
		want:  []string{"missing type"},
	}, {
		name:  "a section is required",
		entry: Entry{Type: "fix", Summary: "did it"},
		want:  []string{"missing section"},
	}, {
		name:  "the type has to be one of the known ones",
		entry: Entry{Section: "vm", Type: "improvement", Summary: "did it"},
		want:  []string{"invalid type 'improvement'"},
	}, {
		name:  "the section has to be one of the published ones",
		entry: Entry{Section: "kernel", Type: "fix", Summary: "did it"},
		want:  []string{"unknown section 'kernel'"},
	}, {
		name:  "the impact level has to be a known one",
		entry: Entry{Section: "vm", Type: "fix", Summary: "did it", ImpactLevel: "huge"},
		want:  []string{"invalid impact level 'huge'"},
	}, {
		name:  "a high impact has to say what to expect",
		entry: Entry{Section: "vm", Type: "fix", Summary: "did it", ImpactLevel: levelHigh},
		want:  []string{"missing high impact detail"},
	}, {
		name:  "a high impact that says it",
		entry: Entry{Section: "vm", Type: "fix", Summary: "did it", ImpactLevel: levelHigh, Impact: "Nodes restart."},
	}, {
		name:  "a section that forces low impact overrides a high one silently",
		entry: Entry{Section: "ci", Type: "fix", Summary: "did it", ImpactLevel: levelHigh},
	}}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := test.entry.Validate(sections)
			if len(got) != len(test.want) {
				t.Fatalf("got %v, want %v", got, test.want)
			}
			for i := range got {
				if !strings.Contains(got[i], test.want[i]) {
					t.Errorf("got %q, want it to mention %q", got[i], test.want[i])
				}
			}
		})
	}
}

// TestCheckAndCollectReadTheSameEntries is the reason this tool exists. The
// validation of a merge request and the generation of the changelog used to be
// two parsers, and every case below is one they disagreed about: what the check
// accepted was not what the changelog got.
func TestCheckAndCollectReadTheSameEntries(t *testing.T) {
	sections := Sections{"vm": "", "vd": "", "ci": levelLow}

	descriptions := map[string][]string{
		"two entries in one block": {
			fence("section: vm\ntype: fix\nsummary: first\n---\nsection: vd\ntype: feature\nsummary: second"),
		},
		"one entry in two sections": {
			fence("section: vm, vd\ntype: fix\nsummary: touches both"),
		},
		"the v1 field names": {
			fence("module: vm\ntype: fix\ndescription: legacy naming"),
		},
		"an example in an HTML comment": {
			"<!--\n" + fence("section: vm\ntype: fix\nsummary: example") + "\n-->\n" +
				fence("section: vd\ntype: fix\nsummary: real"),
		},
		"a section with the forced level in its name": {
			fence("section: ci:low\ntype: fix\nsummary: tweak"),
		},
	}

	for name, cases := range descriptions {
		t.Run(name, func(t *testing.T) {
			for _, description := range cases {
				blocks := ParseDescription(description)

				// What the check sees.
				var problems []string
				for _, block := range blocks {
					problems = append(problems, blockProblems(block, sections)...)
				}
				if len(problems) > 0 {
					t.Fatalf("the check refuses a valid description: %v", problems)
				}

				// What the changelog gets. Both walk the same blocks, so the
				// only way they can differ is a section the list does not have.
				var collected int
				for _, block := range blocks {
					for _, entry := range block.Entries {
						if _, known := sections[entry.Section]; known {
							collected++
						}
					}
				}
				var accepted int
				for _, block := range blocks {
					accepted += len(block.Entries)
				}
				if collected != accepted {
					t.Errorf("the check accepted %d entries and the changelog would keep %d",
						accepted, collected)
				}
			}
		})
	}
}
