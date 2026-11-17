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
	"path/filepath"
	"strings"
	"testing"
)

// Two patch milestones of one minor version are open at once after a patch
// release whose milestone nobody closed. Both write CHANGELOG-v1.12.md in one
// run, and each branch must carry its own block on top of the base only.
func TestMilestonesOfOneMinorVersion(t *testing.T) {
	directory := t.TempDir()

	mustGit := func(args ...string) string {
		t.Helper()

		output, err := git(directory, args...)
		if err != nil {
			t.Fatal(err)
		}

		return output
	}

	mustGit("init", "--quiet")
	mustGit("config", "user.email", "test@example.com")
	mustGit("config", "user.name", "Test")
	mustGit("config", "commit.gpgsign", "false")

	sections := fixtureSections()
	base := MergeMinorMarkdown("", "v1.12", "v1.12.3", RenderMilestoneBlock(fixtureEntries(), sections, "v1.12.3"))

	if err := os.MkdirAll(filepath.Join(directory, "CHANGELOG"), 0o755); err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(directory, "CHANGELOG", "CHANGELOG-v1.12.md"), []byte(base), 0o644); err != nil {
		t.Fatal(err)
	}

	mustGit("add", "CHANGELOG")
	mustGit("commit", "--quiet", "-m", "base")
	baseSHA := mustGit("rev-parse", "HEAD")

	// The order of the scheduled job: the newer milestone comes first.
	for _, milestone := range []string{"v1.12.5", "v1.12.4"} {
		if err := resetToBase(directory, baseSHA); err != nil {
			t.Fatalf("%s: %v", milestone, err)
		}

		ymlPath, markdownPath, err := writeFiles(directory, sections, milestone, collectResult{entries: fixtureEntries()})
		if err != nil {
			t.Fatalf("%s: %v", milestone, err)
		}

		committed, err := commitChangelog(directory, "changelog/"+milestone, milestone, []string{ymlPath, markdownPath})
		if err != nil {
			t.Fatalf("%s: %v", milestone, err)
		}

		if !committed {
			t.Fatalf("%s: nothing was committed", milestone)
		}
	}

	for milestone, other := range map[string]string{"v1.12.5": "v1.12.4", "v1.12.4": "v1.12.5"} {
		markdown := mustGit("show", "changelog/"+milestone+":CHANGELOG/CHANGELOG-v1.12.md")

		for _, want := range []string{"## " + milestone, "## v1.12.3"} {
			if !strings.Contains(markdown, want) {
				t.Errorf("changelog/%s: CHANGELOG-v1.12.md has no %q", milestone, want)
			}
		}

		if strings.Contains(markdown, "## "+other) {
			t.Errorf("changelog/%s: CHANGELOG-v1.12.md carries the block of %s", milestone, other)
		}

		if parent := mustGit("rev-parse", "changelog/"+milestone+"^"); parent != baseSHA {
			t.Errorf("changelog/%s is not cut from the base commit", milestone)
		}
	}
}
