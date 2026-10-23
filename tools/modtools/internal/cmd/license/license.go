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

// Package license implements the `license` command group: `add` prepends the
// Flant license header to files that need one, and `check` fails when a file
// that should carry a header does not, or carries one that matches no known
// license template.
//
// Only the head of a file is inspected (see headerLimit): a mention of
// "Flant" or "Deckhouse" further down the file must not pass for a header.
package license

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/deckhouse/virtualization/tools/modtools/internal/report"
)

func NewCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "license",
		Short: "Manage Flant license headers",
	}

	var addDir, addLicense string
	add := &cobra.Command{
		Use:   "add",
		Short: "Add the Flant license header to files that need one",
		RunE: func(_ *cobra.Command, _ []string) error {
			forceEE, err := parseLicenseFlag(addLicense)
			if err != nil {
				return err
			}
			return report.ExitErr(walk(addDir, true, forceEE))
		},
	}
	add.Flags().StringVar(&addDir, "directory", "./", "Directory to process.")
	add.Flags().StringVar(&addLicense, "license", "ce",
		`License to insert: "ce" (Apache 2.0) or "ee" (Deckhouse Platform Enterprise Edition). `+
			`Go files carrying an EE build constraint get the EE header regardless of this flag.`)

	var checkDir string
	check := &cobra.Command{
		Use:   "check",
		Short: "Verify that files requiring a license header have one",
		RunE: func(_ *cobra.Command, _ []string) error {
			return report.ExitErr(walk(checkDir, false, false))
		},
	}
	check.Flags().StringVar(&checkDir, "directory", "./", "Directory to check.")

	cmd.AddCommand(add, check)
	return cmd
}

// parseLicenseFlag turns the --license value into "insert the EE header".
func parseLicenseFlag(value string) (forceEE bool, err error) {
	switch strings.ToLower(value) {
	case "ce":
		return false, nil
	case "ee":
		return true, nil
	default:
		return false, fmt.Errorf("unknown license %q: expected \"ce\" or \"ee\"", value)
	}
}

// ceText is the Apache 2.0 header carried by everything but Enterprise Edition
// sources. The indentation of the license URL matches the headers already in
// the repository (four spaces, as in the Apache appendix of ./LICENSE).
var ceText = fmt.Sprintf(`Copyright %d Flant JSC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
`, time.Now().Year())

// eeText is the header for Enterprise Edition sources — Apache 2.0 does not
// apply to them. Such files are recognized by their build constraint, see
// goBuildEERe.
var eeText = fmt.Sprintf(`Copyright %d Flant JSC
Licensed under the Deckhouse Platform Enterprise Edition (EE) license. See https://github.com/deckhouse/deckhouse/blob/main/ee/LICENSE
`, time.Now().Year())

func blockComment(text string) string { return "/*\n" + text + "*/\n" }

// hashComment prefixes every line with "# ", leaving bare "#" on blank lines:
// "# " would be trailing whitespace, which .editorconfig trims.
func hashComment(text string) string {
	var b strings.Builder
	for _, line := range strings.Split(strings.TrimRight(text, "\n"), "\n") {
		if line == "" {
			b.WriteString("#\n")
			continue
		}
		b.WriteString("# ")
		b.WriteString(line)
		b.WriteString("\n")
	}
	return b.String()
}

// fileToCheckRe matches files that require a license header:
//   - Ends with .go, .sh, .py
//   - Any file without an extension in its last path segment
//   - Files under .github/{scripts,workflows,workflow_templates} ending with .js
//
// YAML carries no license header: neither the module chart nor the CI
// configuration is source code we license per file, and DMT does not check
// YAML either.
var fileToCheckRe = regexp.MustCompile(`\.go$|/[^/.]+$|\.sh$|\.py$|\.github/(scripts|workflows|workflow_templates)/.+\.js$`)

// fileToSkipRe matches paths that must be skipped:
//   - Non-source trees: .git/, /docs/, vendor/, node_modules/, testdata/, and
//     vendored upstream copies (a /__<dir>/ path segment, e.g.
//     images/*/__kubevirt). The pattern requires a directory so that Python
//     dunder files such as __init__.py are still checked.
//   - Third-party trees excluded from the license rule in .dmtlint.yaml:
//     the vendored docker sources under images/*-artifact/staging and the test
//     SSH keys in test/performance/ssh.
//   - Dockerfile, Makefile, Taskfile, LICENSE, CODEOWNERS
//   - geohash.lua, bashrc, inputrc, modules_menu_skip
var fileToSkipRe = regexp.MustCompile(`geohash\.lua$|\.git/|/vendor/|/node_modules/|/testdata/|/__[^/]+/` +
	`|images/[^/]+-artifact/staging/|test/performance/ssh/` +
	`|CODEOWNERS$|Dockerfile$|Makefile$|Taskfile|/docs/|bashrc$|inputrc$|modules_menu_skip$|LICENSE$`)

// CELicenseRe matches the Apache 2.0 header in any comment style. The year is
// optional: the code generators in api/ use a boilerplate without one
// (api/scripts/boilerplate.go.txt).
var CELicenseRe = regexp.MustCompile(`(?s)[/#{!-]*(\s)*Copyright (20\d{2} )?Flant JSC[-!}\s\n#/]*
[/#{!-]*(\s)*Licensed under the Apache License, Version 2.0 \(the "License"\);[-!}\n]*
[/#{!-]*(\s)*you may not use this file except in compliance with the License.[-!}\n]*
[/#{!-]*(\s)*You may obtain a copy of the License at[-!}\n\s#/]*
[/#{!-]*(\s)*http://www.apache.org/licenses/LICENSE-2.0[-!}\n\s#/]*
[/#{!-]*(\s)*Unless required by applicable law or agreed to in writing, software[-!}\n]*
[/#{!-]*(\s)*distributed under the License is distributed on an "AS IS" BASIS,[-!}\n]*
[/#{!-]*(\s)*WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.[-!}\n]*
[/#{!-]*(\s)*See the License for the specific language governing permissions and[-!}\n]*
[/#{!-]*(\s)*limitations under the License.[-!}\n]{0,1}`)

// EELicenseRe matches the Enterprise Edition header, which is a two-line
// notice instead of the Apache boilerplate.
var EELicenseRe = regexp.MustCompile(`(?s)[/#{!-]*(\s)*Copyright (20\d{2} )?Flant JSC[-!}\s\n#/]*
[/#{!-]*(\s)*Licensed under the Deckhouse Platform Enterprise Edition \(EE\) license\.`)

var (
	// generatedRe marks a file produced by a code generator: those carry the
	// boilerplate of whatever generated them, so no header is required.
	generatedRe = regexp.MustCompile(`(?i)do not (edit|modify)|autogenerated`)
	// ourFileRe tells our own header apart from an upstream one: a file
	// claiming Flant copyright must match a known template.
	ourFileRe   = regexp.MustCompile(`Flant JSC`)
	copyrightRe = regexp.MustCompile(`Copyright`)
	// goBuildEERe detects Enterprise Edition sources by their build constraint.
	goBuildEERe = regexp.MustCompile(`(?m)^//\s*(go:build|\+build).*\bEE\b`)
)

// headerLimit caps how much of a file is inspected. The header sits at the very
// top; reading further only lets unrelated text masquerade as a license.
const headerLimit = 2048

func fileHeader(content []byte) []byte {
	if len(content) > headerLimit {
		return content[:headerLimit]
	}
	return content
}

// licenseForFile returns the header to insert, in the comment style of the
// file's language. The license is CE unless forceEE is set or the file carries
// an EE build constraint. An empty string means the comment style is unknown.
func licenseForFile(filePath string, header []byte, forceEE bool) string {
	text := ceText
	if forceEE || goBuildEERe.Match(header) {
		text = eeText
	}
	switch filepath.Ext(filePath) {
	case ".go", ".js":
		return blockComment(text)
	case ".py", ".sh", ".bash", ".zsh":
		return hashComment(text)
	default:
		return ""
	}
}

type licenseKind string

const (
	kindNone      licenseKind = ""
	kindCE        licenseKind = "CE"
	kindEE        licenseKind = "EE"
	kindGenerated licenseKind = "generated"
	kindForeign   licenseKind = "third-party"
	kindInvalid   licenseKind = "invalid"
)

// detectLicense categorizes the header a file already carries. It must be given
// the file header, not the whole file (see fileHeader).
//
// Order matters: a known template wins over the generated-file marker, so a
// generated file with a proper header still counts as licensed; and the
// "claims Flant copyright" fallback comes last, so that a header we recognize
// is never reported as broken.
func detectLicense(header []byte) licenseKind {
	switch {
	case CELicenseRe.Match(header):
		return kindCE
	case EELicenseRe.Match(header):
		return kindEE
	case generatedRe.Match(header):
		return kindGenerated
	case ourFileRe.Match(header):
		// Our copyright, but the license text matches no known template:
		// a truncated, reworded or hand-edited header.
		return kindInvalid
	case copyrightRe.Match(header):
		return kindForeign
	default:
		return kindNone
	}
}

// result accumulates the outcome of a walk so it can be summarized instead of
// printing one line per file. Only the actionable failures (missing or broken
// headers, read/write errors) keep their paths; everything else is a count.
type result struct {
	scanned     int
	ce          int
	ee          int
	generated   int      // produced by a code generator, header not required
	foreign     int      // upstream copyright (KubeVirt, CDI, docker, ...)
	added       int      // add mode: license inserted
	unsupported int      // no header and unknown comment style (cannot classify)
	missing     []string // known source file with no header
	invalid     []string // claims Flant copyright, matches no known template
	failed      []string // read/write errors ("path: reason")
}

// walk visits dir, categorizing every candidate file. When add is true it
// prepends the license to files that lack one, the EE one when forceEE is set.
// Returns 1 when the run found actionable problems (missing/unsupported/errors).
func walk(dir string, add, forceEE bool) int {
	directory, err := filepath.Abs(dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Cannot get absolute path of directory: %v\n", err)
		return 1
	}
	fmt.Println("Directory:", directory)

	files, err := candidateFiles(directory)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	res := &result{}
	for _, filePath := range files {
		if fileToSkipRe.MatchString(filePath) || !fileToCheckRe.MatchString(filePath) {
			continue
		}
		info, err := os.Lstat(filePath)
		if err != nil || info.IsDir() || !info.Mode().IsRegular() {
			continue
		}
		res.process(filePath, info.Mode().Perm(), add, forceEE)
	}
	res.print(add)
	return res.exitCode()
}

// candidateFiles lists absolute paths under dir that git does not ignore
// (tracked plus untracked-but-not-ignored, so newly created files are still
// covered). It falls back to a full filesystem walk when dir is not inside a
// git work tree.
func candidateFiles(dir string) ([]string, error) {
	out, err := exec.Command("git", "-C", dir, "ls-files", "--cached", "--others", "--exclude-standard", "-z").Output()
	if err != nil {
		return walkAll(dir)
	}
	var files []string
	for _, rel := range strings.Split(strings.TrimRight(string(out), "\x00"), "\x00") {
		if rel != "" {
			files = append(files, filepath.Join(dir, rel))
		}
	}
	return files, nil
}

func walkAll(dir string) ([]string, error) {
	var files []string
	err := filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			files = append(files, p)
		}
		return nil
	})
	return files, err
}

func (r *result) process(filePath string, mode os.FileMode, add, forceEE bool) {
	r.scanned++

	content, err := os.ReadFile(filePath)
	if err != nil {
		r.failed = append(r.failed, fmt.Sprintf("%s: read: %v", filePath, err))
		return
	}

	header := fileHeader(content)
	switch detectLicense(header) {
	case kindCE:
		r.ce++
		return
	case kindEE:
		r.ee++
		return
	case kindGenerated:
		r.generated++
		return
	case kindForeign:
		r.foreign++
		return
	case kindInvalid:
		r.invalid = append(r.invalid, filePath)
		return
	}

	// No header at all.
	licenseFileText := licenseForFile(filePath, header, forceEE)
	if licenseFileText == "" {
		// Unknown comment style (e.g. an extension-less data file): we can
		// neither insert a header nor confidently call it a missing license.
		r.unsupported++
		return
	}
	if !add {
		r.missing = append(r.missing, filePath)
		return
	}
	if err := writeWithLicense(filePath, content, licenseFileText, mode); err != nil {
		r.failed = append(r.failed, fmt.Sprintf("%s: write: %v", filePath, err))
		return
	}
	r.added++
}

// preambleRe matches a line that must stay at the very top of a file, above the
// license: a shebang, or a Go build constraint (gofmt moves //go:build back to
// the first line, so a header inserted above it is not gofmt-clean).
var preambleRe = regexp.MustCompile(`^(#!|//\s*(go:build|\+build)\s)`)

// splitPreamble separates the leading shebang / build constraint lines from the
// rest of the file.
func splitPreamble(content string) (preamble, rest string) {
	lines := strings.SplitAfter(content, "\n")
	n := 0
	for n < len(lines) && preambleRe.MatchString(lines[n]) {
		n++
	}
	if n == 0 {
		return "", content
	}
	return strings.Join(lines[:n], ""), strings.TrimLeft(strings.Join(lines[n:], ""), "\n")
}

// writeWithLicense prepends licenseText to content, keeping any shebang and
// build constraints on top, and preserves the file's original permissions (so
// executable scripts stay +x).
func writeWithLicense(filePath string, content []byte, licenseText string, mode os.FileMode) error {
	var buf bytes.Buffer
	preamble, rest := splitPreamble(string(content))
	if preamble != "" {
		buf.WriteString(preamble)
		buf.WriteString("\n")
		buf.WriteString(licenseText)
		buf.WriteString("\n")
		buf.WriteString(rest)
	} else {
		buf.WriteString(licenseText)
		buf.WriteString("\n")
		buf.WriteString(rest)
	}
	return os.WriteFile(filePath, buf.Bytes(), mode)
}

// exitCode fails only on real problems: source files missing a header (check
// mode), headers that match no known template, and read/write errors.
// Unclassified files are informational.
func (r *result) exitCode() int {
	if len(r.missing)+len(r.invalid)+len(r.failed) > 0 {
		return 1
	}
	return 0
}

func (r *result) print(add bool) {
	printList := func(title string, items []string) {
		if len(items) == 0 {
			return
		}
		fmt.Printf("\n%s (%d):\n", title, len(items))
		for _, it := range items {
			fmt.Printf("  * %s\n", it)
		}
	}

	// Only the actionable failures keep their paths; everything else is a count
	// in the summary below.
	if !add {
		printList("Missing license", r.missing)
	}
	// `add` does not touch a file that already has a header, so a broken one
	// is reported in both modes and has to be fixed by hand.
	printList("License header matches no known template", r.invalid)
	printList("Errors", r.failed)

	fmt.Printf("\nSummary: %d files scanned, %d with CE license", r.scanned, r.ce)
	if r.ee > 0 {
		fmt.Printf(", %d with EE license", r.ee)
	}
	if r.generated > 0 {
		fmt.Printf(", %d generated", r.generated)
	}
	if r.foreign > 0 {
		fmt.Printf(", %d third-party", r.foreign)
	}
	if add {
		fmt.Printf(", %d added", r.added)
	} else {
		fmt.Printf(", %d missing", len(r.missing))
	}
	if len(r.invalid) > 0 {
		fmt.Printf(", %d not matching a known template", len(r.invalid))
	}
	if r.unsupported > 0 {
		fmt.Printf(", %d unclassified", r.unsupported)
	}
	if len(r.failed) > 0 {
		fmt.Printf(", %d errors", len(r.failed))
	}
	fmt.Println(".")
}
