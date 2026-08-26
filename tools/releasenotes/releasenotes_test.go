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
	"unicode/utf8"
)

// The notes of the module itself: the tool has to keep working on the corpus it
// was written for, not only on fixtures.
const realSource = "../../CHANGELOG/release-notes.yaml"

// Regenerate the golden files:
//
//	UPDATE_GOLDEN=1 go test ./tools/releasenotes/
var update = os.Getenv("UPDATE_GOLDEN") != ""

func TestRenderFixture(t *testing.T) {
	releases, err := load("testdata/release-notes.yaml")
	if err != nil {
		t.Fatal(err)
	}

	pages := map[string]string{}
	for _, lang := range []string{"en", "ru"} {
		page, err := renderMarkdown(releases, lang, false)
		if err != nil {
			t.Fatal(err)
		}
		pages[pageName(lang)] = page
	}
	changelog, err := renderChangelog(&releases[0])
	if err != nil {
		t.Fatal(err)
	}
	pages["changelog.yaml"] = changelog

	for name, got := range pages {
		golden := filepath.Join("testdata", name)
		if update {
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

func TestCheckAcceptsTheFixture(t *testing.T) {
	releases, err := load("testdata/release-notes.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if problems := check(releases, ""); len(problems) > 0 {
		t.Errorf("unexpected problems: %v", problems)
	}
}

func TestCheckProblems(t *testing.T) {
	tests := []struct {
		name, source, want string
	}{
		{
			name: "no highlights",
			source: `
- version: v1.0.0
  date: '2026-01-01'
  fixes:
  - en: Fixed it.
    ru: Исправлено.`,
			want: "no highlights",
		},
		{
			name: "too many highlights",
			source: `
- version: v1.0.0
  date: '2026-01-01'
  highlights:
  - {en: One., ru: Один.}
  - {en: Two., ru: Два.}
  - {en: Three., ru: Три.}
  - {en: Four., ru: Четыре.}
  - {en: Five., ru: Пять.}
  - {en: Six., ru: Шесть.}`,
			want: "6 highlights",
		},
		{
			name: "grouped highlights",
			source: `
- version: v1.0.0
  date: '2026-01-01'
  highlights:
    module:
    - {en: One., ru: Один.}`,
			want: "highlights are not grouped",
		},
		{
			name: "item without a translation",
			source: `
- version: v1.0.0
  date: '2026-01-01'
  highlights:
  - {en: One., ru: Один.}
  fixes:
  - en: Fixed it.
    ru: ""`,
			want: "has no ru text",
		},
		{
			name: "item left in English",
			source: `
- version: v1.0.0
  date: '2026-01-01'
  highlights:
  - {en: One., ru: Один.}
  fixes:
  - {en: Fixed it., ru: Fixed it.}`,
			want: "is not translated",
		},
		{
			name: "Russian in the en text",
			source: `
- version: v1.0.0
  date: '2026-01-01'
  highlights:
  - {en: One., ru: Один.}
  fixes:
  - {en: Исправлено., ru: Исправлено.}`,
			want: "Cyrillic in the en text",
		},
		{
			name: "link with a domain",
			source: `
- version: v1.0.0
  date: '2026-01-01'
  highlights:
  - en: See [the docs](https://deckhouse.io/modules/virtualization/).
    ru: См. [документацию](https://deckhouse.io/modules/virtualization/).`,
			want: "a link with a domain",
		},
		{
			name: "an item without the en text",
			source: `
- version: v1.0.0
  date: '2026-01-01'
  highlights:
  - {en: One., ru: Один.}
  fixes:
  - ru: Исправлено.`,
			want: "an item has no en text",
		},
		{
			name: "unknown group",
			source: `
- version: v1.0.0
  date: '2026-01-01'
  highlights:
  - {en: One., ru: Один.}
  fixes:
    storage:
    - {en: Fixed it., ru: Исправлено.}`,
			want: "unknown groups [storage]",
		},
		{
			name: "empty section",
			source: `
- version: v1.0.0
  date: '2026-01-01'
  highlights:
  - {en: One., ru: Один.}
  fixes: []`,
			want: "an empty section",
		},
		{
			name: "empty group",
			source: `
- version: v1.0.0
  date: '2026-01-01'
  highlights:
  - {en: One., ru: Один.}
  fixes:
    module: []
    network:
    - {en: Fixed it., ru: Исправлено.}`,
			want: "an empty group",
		},
		{
			name: "a continued line left flush left",
			source: `
- version: v1.0.0
  date: '2026-01-01'
  highlights:
  - {en: One., ru: Один.}
  upgrade:
  - en: |-
      The first line.
      The second one, flush left.
    ru: |-
      Первая строка.
      Вторая, без отступа.`,
			want: "not indented by two spaces",
		},
		{
			name: "release without a version",
			source: `
- date: '2026-01-01'
  highlights:
  - {en: One., ru: Один.}`,
			want: "no version",
		},
		{
			name: "broken date",
			source: `
- version: v1.0.0
  date: '2026-02-31'
  highlights:
  - {en: One., ru: Один.}`,
			want: "day out of range",
		},
		{
			name: "not a version",
			source: `
- version: 1.0
  date: '2026-01-01'
  highlights:
  - {en: One., ru: Один.}`,
			want: "not a version",
		},
		{
			name: "the same version twice",
			source: `
- version: v1.0.0
  date: '2026-01-02'
  highlights:
  - {en: One., ru: Один.}
- version: v1.0.0
  date: '2026-01-01'
  highlights:
  - {en: One., ru: Один.}`,
			want: "appears twice",
		},
		{
			name: "the oldest release first",
			source: `
- version: v1.0.0
  date: '2026-01-01'
  highlights:
  - {en: One., ru: Один.}
- version: v1.1.0
  date: '2026-01-02'
  highlights:
  - {en: One., ru: Один.}`,
			want: "from the newest to the oldest",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			releases := decode(t, test.source)
			problems := strings.Join(check(releases, ""), "\n")
			if !strings.Contains(problems, test.want) {
				t.Errorf("expected a problem about %q, got:\n%s", test.want, problems)
			}
		})
	}
}

// The changelog goes into the release image, so it must be the newest release even
// when the file is out of order — check reports the order separately.
func TestNewestIsUsedForTheChangelog(t *testing.T) {
	releases := decode(t, `
- version: v1.0.0
  date: '2026-01-01'
  highlights:
  - {en: Old one., ru: Старый.}
- version: v1.2.0
  date: '2026-02-01'
  highlights:
  - {en: New one., ru: Новый.}`)

	changelog, err := renderChangelog(newest(releases))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(changelog, "New one.") {
		t.Errorf("the changelog is not the newest release:\n%s", changelog)
	}
}

// print puts one release into an announcement: no front matter, the requested
// release only, absolute links when asked.
func TestPrintRelease(t *testing.T) {
	releases := decode(t, `
- version: v1.1.0
  date: '2026-02-01'
  highlights:
  - {en: New one., ru: Новый.}
- version: v1.0.0
  date: '2026-01-01'
  highlights:
  - en: See [VirtualDisk](/modules/virtualization/cr.html#virtualdisk).
    ru: См. [VirtualDisk](/modules/virtualization/cr.html#virtualdisk).`)

	notes, err := printRelease(releases, "", "ru", false)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(notes, "## v1.1.0") || strings.Contains(notes, "v1.0.0") {
		t.Errorf("expected only the newest release, got:\n%s", notes)
	}

	notes, err = printRelease(releases, "v1.0.0", "ru", true)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(notes, "](https://deckhouse.ru/modules/virtualization/cr.html#virtualdisk)") {
		t.Errorf("expected the chosen release with absolute links, got:\n%s", notes)
	}

	if _, err := printRelease(releases, "v9.9.9", "ru", false); err == nil ||
		!strings.Contains(err.Error(), "no release v9.9.9") {
		t.Errorf("expected the unknown version to be refused, got %v", err)
	}
}

// The --links flag renders the pages for pasting outside the documentation, where
// a path leads nowhere: every link gets the site domain of its language.
func TestMarkdownWithAbsoluteLinks(t *testing.T) {
	releases := decode(t, `
- version: v1.0.0
  date: '2026-01-01'
  highlights:
  - en: Added [VirtualMachine](/modules/virtualization/cr.html#virtualmachine).
    ru: Добавлен [VirtualMachine](/modules/virtualization/cr.html#virtualmachine).`)

	for lang, base := range map[string]string{"en": docBaseURL, "ru": docBaseURLRu} {
		page, err := renderMarkdown(releases, lang, true)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(page, "]("+base+"/modules/virtualization/cr.html#virtualmachine)") {
			t.Errorf("%s: expected links on the domain %s, got:\n%s", lang, base, page)
		}
	}
}

// A release build passes its tag to render; publishing the changelog of a foreign
// version in the Console must fail the build, not slip through.
func TestRenderRefusesAForeignVersion(t *testing.T) {
	releases := decode(t, `
- version: v1.1.0
  date: '2026-01-01'
  highlights:
  - {en: One., ru: Один.}`)

	dir := t.TempDir()
	err := render(releases, dir, "v1.0.9", false)
	if err == nil || !strings.Contains(err.Error(), "v1.1.0") {
		t.Errorf("expected the foreign version to be refused, got %v", err)
	}
	if err := render(releases, dir, "v1.1.0", false); err != nil {
		t.Fatal(err)
	}
	if err := render(releases, dir, "", false); err != nil {
		t.Fatal(err)
	}
}

// A `|` block scalar keeps a trailing newline; it must not become an empty list
// item in the page or a trailing break in the Console.
func TestKeptNewlineIsTrimmed(t *testing.T) {
	releases := decode(t, `
- version: v1.0.0
  date: '2026-01-01'
  highlights:
  - en: |
      One.
    ru: |
      Один.`)

	if got := releases[0].Highlights.Items[0].En; got != "One." {
		t.Errorf("expected the trailing newline to be trimmed, got %q", got)
	}
	page, err := renderMarkdown(releases, "en", false)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(page, "- One.\n\n\n") {
		t.Errorf("an empty line leaked into the list:\n%s", page)
	}
}

// A key left without a value decodes to a nil pointer without an error, so it is
// rejected separately: a dangling `fixes:` in the source is dirt, not an empty
// section.
func TestDanglingKeyIsRejected(t *testing.T) {
	path := filepath.Join(t.TempDir(), "release-notes.yaml")
	source := `
- version: v1.0.0
  date: '2026-01-01'
  highlights:
  - {en: One., ru: Один.}
  fixes:`
	if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := load(path)
	if err == nil || !strings.Contains(err.Error(), "has no value") {
		t.Errorf("expected the dangling key to be rejected, got %v", err)
	}
}

// KnownFields of the top-level decoder does not survive node.Decode inside a
// custom unmarshaller, so the strictness at the item level is Item's own.
func TestUnknownKeyInANoteIsRejected(t *testing.T) {
	tests := []struct {
		name   string
		source string
	}{
		{
			name: "in a flat section",
			source: `
- version: v1.0.0
  date: '2026-01-01'
  highlights:
  - {en: One., ru: Один., pull_request: https://example.com/1}`,
		},
		{
			name: "in a group",
			source: `
- version: v1.0.0
  date: '2026-01-01'
  highlights:
  - {en: One., ru: Один.}
  fixes:
    module:
    - {en: Fixed., ru: Исправлено., typo_key: oops}`,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "release-notes.yaml")
			if err := os.WriteFile(path, []byte(test.source), 0o644); err != nil {
				t.Fatal(err)
			}
			_, err := load(path)
			if err == nil || !strings.Contains(err.Error(), "unknown key") {
				t.Errorf("expected the unknown key in a note to be rejected, got %v", err)
			}
		})
	}
}

// The typical structural slip: a note written straight under the section, where a
// group key is expected.
func TestItemWithoutAGroupIsRejected(t *testing.T) {
	path := filepath.Join(t.TempDir(), "release-notes.yaml")
	source := `
- version: v1.0.0
  date: '2026-01-01'
  highlights:
  - {en: One., ru: Один.}
  fixes:
    en: Fixed it.
    ru: Исправлено.`
	if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := load(path)
	if err == nil || !strings.Contains(err.Error(), "holds a list of items") {
		t.Errorf("expected a readable message about the missing group, got %v", err)
	}
}

// A note that carries ": " and is not quoted breaks the file; the parser's own
// message does not say why, so the tool adds the reason.
func TestColonInAnUnquotedNoteIsExplained(t *testing.T) {
	path := filepath.Join(t.TempDir(), "release-notes.yaml")
	source := "- version: v1.0.0\n  date: '2026-01-01'\n  highlights:\n  - en: Broken text: with a colon.\n    ru: Сломанный текст.\n"
	if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := load(path)
	if err == nil || !strings.Contains(err.Error(), "has to be quoted") {
		t.Errorf("expected the colon to be explained, got %v", err)
	}
}

// A vulnerability fix is announced in every release that carries it, so a release
// whose changelog lists one and whose notes do not name it is a problem.
func TestAdvisoryMissingFromTheNotes(t *testing.T) {
	dir := t.TempDir()
	changelog := "core:\n  fixes:\n    - summary: Fixed vulnerabilities CVE-2026-1000 and CVE-2026-2000.\n" +
		"    - summary: Reworked GO-based tooling and GHSA- prefix handling, GO-2026-3000 remains.\n"
	if err := os.WriteFile(filepath.Join(dir, "CHANGELOG-v1.0.0.yml"), []byte(changelog), 0o644); err != nil {
		t.Fatal(err)
	}

	releases := decode(t, `
- version: v1.0.0
  date: '2026-01-01'
  highlights:
  - {en: One., ru: Один.}
  security:
    module:
    - en: Fixed vulnerabilities CVE-2026-1000.
      ru: Исправлены уязвимости CVE-2026-1000.`)

	problems := strings.Join(check(releases, dir), "\n")
	for _, id := range []string{"CVE-2026-2000", "GO-2026-3000"} {
		if !strings.Contains(problems, id) {
			t.Errorf("expected the unannounced vulnerability %s to be reported, got:\n%s", id, problems)
		}
	}
	if strings.Contains(problems, "CVE-2026-1000") {
		t.Errorf("the announced vulnerability must not be reported:\n%s", problems)
	}
	// Words that only look like advisories must not count as ones.
	if strings.Contains(problems, "GO-based") || strings.Contains(problems, "GHSA-") {
		t.Errorf("a word is mistaken for an advisory:\n%s", problems)
	}
	if problems := check(releases, ""); len(problems) > 0 {
		t.Errorf("without a changelog directory there is nothing to compare: %v", problems)
	}
}

// The problem reports quote the text of an item; a Russian text longer than the
// limit must be cut between the letters, not in the middle of one.
func TestCutKeepsRunesWhole(t *testing.T) {
	long := strings.Repeat("я", 200)
	if got := cut(long); !utf8.ValidString(got) || got != strings.Repeat("я", 110) {
		t.Errorf("expected 110 whole letters, got %q", got)
	}
	if got := cut("short\nrest"); got != "short" {
		t.Errorf("expected the first line, got %q", got)
	}
}

func TestUnknownSectionIsRejected(t *testing.T) {
	path := filepath.Join(t.TempDir(), "release-notes.yaml")
	source := `
- version: v1.0.0
  date: '2026-01-01'
  highlights:
  - {en: One., ru: Один.}
  performance:
  - {en: Faster., ru: Быстрее.}`
	if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := load(path)
	if err == nil || !strings.Contains(err.Error(), "performance") {
		t.Errorf("expected the unknown section to be rejected, got %v", err)
	}
}

func TestChangelogLinksAreAbsolute(t *testing.T) {
	releases := decode(t, `
- version: v1.0.0
  date: '2026-01-01'
  highlights:
  - en: Added the [VirtualMachine](/modules/virtualization/cr.html#virtualmachine) resource.
    ru: Добавлен ресурс [VirtualMachine](/modules/virtualization/cr.html#virtualmachine).`)

	changelog, err := renderChangelog(&releases[0])
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(changelog, "](https://deckhouse.io/modules/virtualization/cr.html#virtualmachine)") {
		t.Errorf("the Console changelog needs absolute links:\n%s", changelog)
	}

	page, err := renderMarkdown(releases, "en", false)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(page, "https://deckhouse.io") {
		t.Errorf("the documentation links by path:\n%s", page)
	}
}

// The module's own notes: they must stay renderable and valid.
func TestRealSource(t *testing.T) {
	if _, err := os.Stat(realSource); os.IsNotExist(err) {
		t.Skipf("no %s", realSource)
	}
	releases, err := load(realSource)
	if err != nil {
		t.Fatal(err)
	}
	if problems := check(releases, filepath.Dir(realSource)); len(problems) > 0 {
		t.Errorf("%s has problems:\n%s", realSource, strings.Join(problems, "\n"))
	}
	for _, lang := range []string{"en", "ru"} {
		page, err := renderMarkdown(releases, lang, false)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.HasPrefix(page, "---\ntitle: ") || !strings.HasSuffix(page, "\n") {
			t.Errorf("%s page does not look like a documentation page", lang)
		}
	}
	if _, err := renderChangelog(&releases[0]); err != nil {
		t.Fatal(err)
	}
}

func decode(t *testing.T, source string) []Release {
	t.Helper()
	path := filepath.Join(t.TempDir(), "release-notes.yaml")
	if err := os.WriteFile(path, []byte(source), 0o644); err != nil {
		t.Fatal(err)
	}
	releases, err := load(path)
	if err != nil {
		t.Fatal(err)
	}
	return releases
}
