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
	"os"
	"strings"
	"testing"
)

// The template every merge request of the project starts from. Its example is
// what the regular expressions this tool replaces kept reading as a real entry.
const mergeRequestTemplate = "../../.gitlab/merge_request_templates/Default.md"

func fence(body string) string {
	return "```changes\n" + body + "\n```"
}

// TestBlocksIgnoresWhatIsNotACodeBlock covers the cases a regular expression
// cannot tell apart from a real entry.
func TestBlocksIgnoresWhatIsNotACodeBlock(t *testing.T) {
	tests := []struct {
		name        string
		description string
		want        []string
	}{{
		name: "a block inside an HTML comment is an example, not an entry",
		description: "<!--\n" + fence("section: vm\ntype: fix\nsummary: example") + "\n-->\n\n" +
			fence("section: vd\ntype: fix\nsummary: real"),
		want: []string{"section: vd\ntype: fix\nsummary: real\n"},
	}, {
		name: "an indented example does not swallow the entry under it",
		description: "Example:\n\n   " + strings.ReplaceAll(
			fence("section: vm\ntype: fix\nsummary: example"), "\n", "\n   ") + "\n\n" +
			fence("section: vd\ntype: fix\nsummary: real"),
		want: []string{
			"section: vm\ntype: fix\nsummary: example\n",
			"section: vd\ntype: fix\nsummary: real\n",
		},
	}, {
		name:        "a block quoted inside a longer fence is documentation",
		description: "````\n" + fence("section: vm\ntype: fix\nsummary: quoted") + "\n````",
		want:        nil,
	}, {
		name:        "an indented code block is not a fenced one",
		description: "    ```changes\n    section: vm\n    type: fix\n    summary: indented\n    ```",
		want:        nil,
	}, {
		name:        "a tilde fence is a fence",
		description: "~~~changes\nsection: vd\ntype: fix\nsummary: tildes\n~~~",
		want:        []string{"section: vd\ntype: fix\nsummary: tildes\n"},
	}, {
		name:        "the info string may carry more than the language",
		description: "```changes linenums\nsection: vd\ntype: fix\nsummary: attributes\n```",
		want:        []string{"section: vd\ntype: fix\nsummary: attributes\n"},
	}, {
		name: "several blocks are read in the order they are written",
		description: fence("section: vm\ntype: fix\nsummary: first") + "\n\n" +
			fence("section: vd\ntype: fix\nsummary: second"),
		want: []string{
			"section: vm\ntype: fix\nsummary: first\n",
			"section: vd\ntype: fix\nsummary: second\n",
		},
	}, {
		name:        "a description without a block has nothing to read",
		description: "Just a description.\n\n```yaml\nsection: vm\n```",
		want:        nil,
	}}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got := Blocks(test.description)
			if len(got) != len(test.want) {
				t.Fatalf("got %d block(s), want %d: %q", len(got), len(test.want), got)
			}
			for i := range got {
				if got[i] != test.want[i] {
					t.Errorf("block %d:\n got: %q\nwant: %q", i+1, got[i], test.want[i])
				}
			}
		})
	}
}

// TestTemplateExampleIsNotAnEntry is the case that put six entries of the
// template's own text into CHANGELOG-v1.11.0: the example lives in an HTML
// comment of the template and stays there in most merge requests.
func TestTemplateExampleIsNotAnEntry(t *testing.T) {
	template, err := os.ReadFile(mergeRequestTemplate)
	if err != nil {
		t.Fatal(err)
	}
	description := strings.Replace(string(template),
		"```changes\nsection:\ntype:\nsummary:\n```",
		fence("section: vd\ntype: fix\nsummary: what the author wrote"), 1)

	blocks := ParseDescription(description)
	if len(blocks) != 1 {
		t.Fatalf("got %d block(s) in the template, want 1", len(blocks))
	}
	if blocks[0].Err != nil {
		t.Fatalf("the block of the template does not parse: %v", blocks[0].Err)
	}
	if len(blocks[0].Entries) != 1 || blocks[0].Entries[0].Summary != "what the author wrote" {
		t.Errorf("got %+v, want the entry of the author", blocks[0].Entries)
	}
}

// TestTemplateStubIsRefused keeps the check failing on a merge request whose
// changelog block was never filled in.
func TestTemplateStubIsRefused(t *testing.T) {
	blocks := ParseDescription(fence("section:\ntype:\nsummary:"))
	if len(blocks) != 1 || blocks[0].Err != nil {
		t.Fatalf("the stub of the template does not parse: %+v", blocks)
	}
	problems := blockProblems(blocks[0], Sections{"vm": ""})
	for _, want := range []string{"missing section", "missing summary", "missing type"} {
		if !containsSubstring(problems, want) {
			t.Errorf("got %v, want it to report %q", problems, want)
		}
	}
}

func TestParseBlock(t *testing.T) {
	tests := []struct {
		name  string
		block string
		want  []Entry
	}{{
		name:  "the keys of an entry",
		block: "section: vm\ntype: fix\nsummary: did it\n",
		want:  []Entry{{Section: "vm", Type: "fix", Summary: "did it"}},
	}, {
		name:  "a block holds one entry per document",
		block: "section: vm\ntype: fix\nsummary: first\n---\nsection: vd\ntype: feature\nsummary: second\n",
		want: []Entry{
			{Section: "vm", Type: "fix", Summary: "first"},
			{Section: "vd", Type: "feature", Summary: "second"},
		},
	}, {
		name:  "a comma-separated section writes the entry into each of them",
		block: "section: vm, vd\ntype: fix\nsummary: touches both\n",
		want: []Entry{
			{Section: "vm", Type: "fix", Summary: "touches both"},
			{Section: "vd", Type: "fix", Summary: "touches both"},
		},
	}, {
		name:  "the v1 names are still read",
		block: "module: vm\ntype: fix\ndescription: legacy naming\nnote: legacy note\n",
		want:  []Entry{{Section: "vm", Type: "fix", Summary: "legacy naming", Impact: "legacy note"}},
	}, {
		name:  "a section keeps the forced level out of its name",
		block: "section: ci:low\ntype: fix\nsummary: tweak\n",
		want:  []Entry{{Section: "ci", Type: "fix", Summary: "tweak"}},
	}, {
		name:  "a quoted summary loses its quotes",
		block: "section: vd\ntype: feature\nsummary: \"Creating a disk is faster.\"\n",
		want:  []Entry{{Section: "vd", Type: "feature", Summary: "Creating a disk is faster."}},
	}, {
		name:  "a single-quoted summary loses its quotes too",
		block: "section: vd\ntype: feature\nsummary: 'it''s faster'\n",
		want:  []Entry{{Section: "vd", Type: "feature", Summary: "it's faster"}},
	}, {
		name:  "a literal block keeps its line breaks and not its indicator",
		block: "section: core\ntype: chore\nsummary: |\n  Fixed vulnerabilities:\n  - CVE-2026-46600\n  - CVE-2025-27144\n",
		want: []Entry{{
			Section: "core", Type: "chore",
			Summary: "Fixed vulnerabilities:\n- CVE-2026-46600\n- CVE-2025-27144",
		}},
	}, {
		name:  "a literal block with a chomping indicator",
		block: "section: core\ntype: chore\nsummary: |-\n  one line\n",
		want:  []Entry{{Section: "core", Type: "chore", Summary: "one line"}},
	}, {
		name:  "a key-looking line inside a literal block stays inside it",
		block: "section: core\ntype: chore\nsummary: |\n  impact: none\n  detail: two\nimpact: real note\n",
		want: []Entry{{
			Section: "core", Type: "chore",
			Summary: "impact: none\ndetail: two", Impact: "real note",
		}},
	}, {
		name:  "a multi-line impact note",
		block: "section: core\ntype: feature\nsummary: containerd v2\nimpact: |\n  First line.\n  Second line.\nimpact_level: high\n",
		want: []Entry{{
			Section: "core", Type: "feature", Summary: "containerd v2",
			Impact: "First line.\nSecond line.", ImpactLevel: "high",
		}},
	}, {
		name:  "a trailing separator carries no entry",
		block: "section: vm\ntype: fix\nsummary: only one\n---\n",
		want:  []Entry{{Section: "vm", Type: "fix", Summary: "only one"}},
	}, {
		name:  "an unknown key is ignored",
		block: "section: vm\ntype: fix\nsummary: did it\nauthor: someone\n",
		want:  []Entry{{Section: "vm", Type: "fix", Summary: "did it"}},
	}}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := ParseBlock(test.block)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(got) != len(test.want) {
				t.Fatalf("got %d entries, want %d: %+v", len(got), len(test.want), got)
			}
			for i := range got {
				if got[i] != test.want[i] {
					t.Errorf("entry %d:\n got: %+v\nwant: %+v", i+1, got[i], test.want[i])
				}
			}
		})
	}
}

// TestParseBlockReportsWhatItCannotRead: a block that is not valid YAML has to
// say so. Reading it half way is how an entry turns into somebody else's text.
func TestParseBlockReportsWhatItCannotRead(t *testing.T) {
	tests := []struct {
		name  string
		block string
		want  string
	}{{
		name:  "a colon in an unquoted summary",
		block: "section: vm\ntype: fix\nsummary: fix: the thing\n",
		want:  "mapping values are not allowed",
	}, {
		name:  "a note continued on the next line without a block scalar",
		block: "section: core\ntype: feature\nsummary: containerd v2\nimpact: First line.\nSecond line.\n",
		want:  "could not find expected ':'",
	}, {
		name:  "an entry that is not a set of keys",
		block: "- section: vm\n- type: fix\n",
		want:  "must be written as `key: value` lines",
	}, {
		name:  "a value of the wrong shape",
		block: "section: vm\ntype: fix\nsummary:\n  - one\n  - two\n",
		want:  "cannot unmarshal",
	}, {
		name:  "the second entry of a block",
		block: "section: vm\ntype: fix\nsummary: fine\n---\nsection: vd\nsummary: broken: here\n",
		want:  "entry #2",
	}}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			_, err := ParseBlock(test.block)
			if err == nil {
				t.Fatal("got no error, want one")
			}
			if !strings.Contains(err.Error(), test.want) {
				t.Errorf("got %q, want it to mention %q", err, test.want)
			}
			if strings.Contains(err.Error(), "rawEntry") {
				t.Errorf("the message names a Go type: %q", err)
			}
		})
	}
}

// TestOneBrokenBlockDoesNotHideTheOthers: the entries of the blocks around a
// broken one are still read, and the broken one is still reported.
func TestOneBrokenBlockDoesNotHideTheOthers(t *testing.T) {
	description := fence("section: vm\ntype: fix\nsummary: first") + "\n\n" +
		fence("section: vd\ntype: fix\nsummary: broken: here") + "\n\n" +
		fence("section: cli\ntype: fix\nsummary: third")

	blocks := ParseDescription(description)
	if len(blocks) != 3 {
		t.Fatalf("got %d blocks, want 3", len(blocks))
	}
	if blocks[0].Err != nil || blocks[2].Err != nil {
		t.Errorf("a readable block was reported as broken: %v, %v", blocks[0].Err, blocks[2].Err)
	}
	if blocks[1].Err == nil {
		t.Error("the broken block was not reported")
	}
	if len(blocks[0].Entries) != 1 || len(blocks[2].Entries) != 1 {
		t.Errorf("the readable blocks lost their entries: %+v", blocks)
	}
}

func containsSubstring(values []string, want string) bool {
	for _, value := range values {
		if strings.Contains(value, want) {
			return true
		}
	}
	return false
}
