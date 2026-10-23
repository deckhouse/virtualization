/*
Copyright 2024 Flant JSC

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

// Package diff parses unified git diff output into a structured form the checks
// can iterate over, and loads it either from a patch file or from `git diff`.
package diff

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"regexp"
	"strings"
)

// Loader returns the parsed diff a command should operate on.
type Loader func() (*DiffInfo, error)

type DiffInfo struct {
	Files []*DiffFileInfo
}

func NewDiffInfo() *DiffInfo {
	return &DiffInfo{Files: make([]*DiffFileInfo, 0)}
}

func (d *DiffInfo) Dump() string {
	res := ""
	for _, info := range d.Files {
		res += fmt.Sprintf("%s -> %s, lines: %d\n", info.OldFileName, info.NewFileName, len(info.Lines))
	}
	res += fmt.Sprintf("files: %d\n", len(d.Files))
	return res
}

type DiffFileInfo struct {
	NewFileName string
	OldFileName string
	Lines       []string
}

func (d *DiffFileInfo) IsAdded() bool {
	return d.OldFileName == "/dev/null"
}

func (d *DiffFileInfo) IsDeleted() bool {
	return d.NewFileName == "/dev/null"
}

func (d *DiffFileInfo) IsModified() bool {
	return d.OldFileName != "/dev/null" && d.NewFileName != "/dev/null" && d.HasContent()
}

func (d *DiffFileInfo) HasContent() bool {
	return len(d.Lines) > 0
}

func (d *DiffFileInfo) NewLines() []string {
	res := make([]string, 0)
	for _, l := range d.Lines {
		if strings.HasPrefix(l, "+") {
			res = append(res, strings.TrimPrefix(l, "+"))
		}
	}
	return res
}

func NewDiffFileInfo() *DiffFileInfo {
	return &DiffFileInfo{Lines: make([]string, 0)}
}

// Load reads the diff from patchFile when set, otherwise from `git diff`.
func Load(patchFile string) (*DiffInfo, error) {
	if patchFile != "" {
		content, err := os.ReadFile(patchFile)
		if err != nil {
			return nil, err
		}
		return ParseDiffOutput(bytes.NewReader(content))
	}

	diffRange := resolveDiffRange(os.Getenv)
	fmt.Printf("Run git diff %s ...\n", diffRange)
	out, err := exec.Command("git", "diff", diffRange, "-w", "--ignore-blank-lines").Output()
	if err != nil {
		return nil, err
	}
	return ParseDiffOutput(bytes.NewReader(out))
}

const zeroSHA = "0000000000000000000000000000000000000000"

// resolveDiffRange returns the git revision range covering the change under
// validation. The range must not depend on a hardcoded base branch: on a
// release branch, a diff against origin/main spans everything backported since
// the branch point, so unrelated files get reported as changed.
//
//  1. Merge request pipeline: three-dot diff against the target branch, so only
//     the commits of this change are validated.
//  2. Push pipeline: the commits actually pushed. There is no target branch
//     here, and a three-dot diff against the branch itself would be empty.
//  3. Anything else (a new branch, a local run): three-dot diff against
//     origin/main.
func resolveDiffRange(getenv func(string) string) string {
	if target := getenv("CI_MERGE_REQUEST_TARGET_BRANCH_NAME"); target != "" {
		return "origin/" + target + "..."
	}
	if before := getenv("CI_COMMIT_BEFORE_SHA"); before != "" && before != zeroSHA {
		return before + "..HEAD"
	}
	return "origin/main..."
}

var (
	diffStartRe   = regexp.MustCompile(`^diff --git a/(.*) b/(.*)$`)
	oldFileNameRe = regexp.MustCompile(`^--- (/dev/null|a/(.*))$`)
	newFileNameRe = regexp.MustCompile(`^\+\+\+ (/dev/null|b/(.*))$`)
	endMetadataRe = regexp.MustCompile(`^@@[\-+ \d,]+@@(.*)$`)
)

func ParseDiffOutput(r io.Reader) (*DiffInfo, error) {
	res := NewDiffInfo()
	tmp := NewDiffFileInfo()
	firstLine := true
	scanner := bufio.NewScanner(r)
	metadataBlock := false
	for scanner.Scan() {
		text := scanner.Text()

		if diffStartRe.MatchString(text) {
			if firstLine {
				firstLine = false
			} else {
				// Append diffFileInfo when all lines are gathered and new diffFIleInfo is detected.
				res.Files = append(res.Files, tmp)
				tmp = NewDiffFileInfo()
			}
			metadataBlock = true
			continue
		}

		matches := newFileNameRe.FindStringSubmatch(text)
		if len(matches) > 1 {
			if matches[1] == "/dev/null" {
				tmp.NewFileName = matches[1]
			} else {
				tmp.NewFileName = matches[2]
			}
			continue
		}

		matches = oldFileNameRe.FindStringSubmatch(text)
		if len(matches) > 1 {
			if matches[1] == "/dev/null" {
				tmp.OldFileName = matches[1]
			} else {
				tmp.OldFileName = matches[2]
			}
			continue
		}

		if metadataBlock {
			matches = endMetadataRe.FindStringSubmatch(text)
			if len(matches) > 1 {
				tmp.Lines = append(tmp.Lines, matches[1])
				metadataBlock = false
				continue
			}
		}

		if !metadataBlock {
			tmp.Lines = append(tmp.Lines, text)
		}
	}
	// Push last diff info.
	if tmp != nil {
		res.Files = append(res.Files, tmp)
	}

	return res, nil
}
