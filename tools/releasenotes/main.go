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

// release-notes turns CHANGELOG/release-notes.yaml, where the notes of every
// release are written in both languages, into the files that are published: the
// two documentation pages and the changelog the Console shows. The werf build
// renders them into the module image, so they are not kept in the repository.
//
//	release-notes -type render -out <dir>   check, then write docs/RELEASE_NOTES{,.ru}.md
//	                                        and changelog.yaml
//	release-notes -type render -links ...   the same pages with absolute links, for
//	                                        pasting outside the documentation
//	release-notes -type print               one release to stdout: the newest, or
//	                                        -version vX.Y.Z; -lang en|ru, -links
//	release-notes -type check               validate the source
package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

const defaultSource = "CHANGELOG/release-notes.yaml"

func main() {
	var runType string
	flag.StringVar(&runType, "type", "", "Run type: render or check.")
	var source string
	flag.StringVar(&source, "source", defaultSource, "Release notes source.")
	var out string
	flag.StringVar(&out, "out", "", "Directory render writes the published files into.")
	var version string
	flag.StringVar(&version, "version", "",
		"For render, the version the build publishes: it fails when it is not the "+
			"newest release in the source. For print, the release to print.")
	var links bool
	flag.BoolVar(&links, "links", false,
		"Render the documentation pages with absolute links (deckhouse.io for en, "+
			"deckhouse.ru for ru), for pasting outside the documentation.")
	var lang string
	flag.StringVar(&lang, "lang", "ru", "The language print uses: en or ru.")
	flag.Parse()

	switch runType {
	case "render", "check", "print":
	default:
		fmt.Printf("Unknown run type '%s'\n", runType)
		flag.Usage()
		os.Exit(2)
	}

	releases, err := load(source)
	if err != nil {
		fmt.Printf("release-notes: %v\n", err)
		os.Exit(1)
	}

	switch runType {
	case "render":
		// A source that fails the checks must not reach the published files, no
		// matter where render is called from.
		if err = runCheck(releases, source); err == nil {
			err = render(releases, out, version, links)
		}
	case "print":
		var notes string
		if notes, err = printRelease(releases, version, lang, links); err == nil {
			fmt.Print(notes)
		}
	case "check":
		err = runCheck(releases, source)
	}
	if err != nil {
		fmt.Printf("release-notes: %v\n", err)
		os.Exit(1)
	}
}

// render writes what the module image carries: the documentation page of both
// languages and the changelog the Console reads.
func render(releases []Release, out, version string, links bool) error {
	// No default: rendering into the repository by accident would leave files
	// behind that are meant to exist only inside the module image.
	if out == "" {
		return fmt.Errorf("render needs -out <dir>")
	}
	latest := newest(releases)
	// The changelog the image carries is the newest release of the source; a build
	// that publishes a different version would show the notes of a foreign release
	// in the Console.
	if version != "" && latest.Version != version {
		return fmt.Errorf("the newest release in the source is %s, and the build "+
			"publishes %s: its notes go into the source before the tag is cut",
			latest.Version, version)
	}
	if err := os.MkdirAll(filepath.Join(out, "docs"), 0o755); err != nil {
		return err
	}
	for _, lang := range []string{"en", "ru"} {
		page, err := renderMarkdown(releases, lang, links)
		if err != nil {
			return err
		}
		if err := os.WriteFile(filepath.Join(out, "docs", pageName(lang)), []byte(page), 0o644); err != nil {
			return err
		}
	}

	changelog, err := renderChangelog(latest)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(out, "changelog.yaml"), []byte(changelog), 0o644)
}

// printRelease returns the notes of one release — the newest one when version is
// empty — as the markdown of one language, without the front matter of the page:
// this is what goes into an announcement, not into the documentation.
func printRelease(releases []Release, version, lang string, links bool) (string, error) {
	if lang != "en" && lang != "ru" {
		return "", fmt.Errorf("the language is en or ru, not %q", lang)
	}
	release := newest(releases)
	if version != "" {
		release = nil
		for i := range releases {
			if releases[i].Version == version {
				release = &releases[i]
				break
			}
		}
		if release == nil {
			return "", fmt.Errorf("no release %s in the source", version)
		}
	}
	page, err := renderMarkdown([]Release{*release}, lang, links)
	if err != nil {
		return "", err
	}
	// The front matter ends at the second ---; the notes start after it.
	_, notes, found := strings.Cut(page, "\n---\n\n")
	if !found {
		return "", fmt.Errorf("the rendered page has no front matter to strip")
	}
	return notes, nil
}

func pageName(lang string) string {
	if lang == "ru" {
		return "RELEASE_NOTES.ru.md"
	}
	return "RELEASE_NOTES.md"
}

func runCheck(releases []Release, source string) error {
	// The generated per-release changelogs live next to the source; they are what
	// the vulnerability check compares against.
	problems := check(releases, filepath.Dir(source))
	// Rendering is the last gate: it fails on anything the checks above cannot see,
	// so a source they accept can still not reach the published files silently. It
	// runs only on a clean source, otherwise every renderer repeats the same
	// complaint.
	if len(problems) == 0 {
		for _, lang := range []string{"en", "ru"} {
			if _, err := renderMarkdown(releases, lang, false); err != nil {
				problems = append(problems, err.Error())
			}
		}
		if _, err := renderChangelog(newest(releases)); err != nil {
			problems = append(problems, err.Error())
		}
	}

	for _, problem := range problems {
		fmt.Printf("release-notes: %s\n", problem)
	}
	switch len(problems) {
	case 0:
		fmt.Printf("ok: %d releases in %s\n", len(releases), source)
		return nil
	case 1:
		return fmt.Errorf("1 problem in %s", source)
	default:
		return fmt.Errorf("%d problems in %s", len(problems), source)
	}
}
