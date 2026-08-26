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
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Item is one note. Both languages live in the same node, so they cannot drift
// apart: there is no way to write a different number of items or to put them in
// a different order in one language.
type Item struct {
	En string `yaml:"en"`
	Ru string `yaml:"ru"`
}

// UnmarshalYAML rejects unknown keys itself: the strictness of the top-level
// decoder (KnownFields) does not survive a node.Decode call inside a custom
// unmarshaller, so without this a misspelled key in a note passes silently.
func (i *Item) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind != yaml.MappingNode {
		return fmt.Errorf("line %d: a note is a mapping of en and ru", node.Line)
	}
	seen := map[string]bool{}
	for j := 0; j+1 < len(node.Content); j += 2 {
		key, value := node.Content[j], node.Content[j+1]
		if seen[key.Value] {
			return fmt.Errorf("line %d: %q appears twice in one note", key.Line, key.Value)
		}
		seen[key.Value] = true
		var target *string
		switch key.Value {
		case "en":
			target = &i.En
		case "ru":
			target = &i.Ru
		default:
			return fmt.Errorf("line %d: unknown key %q, a note holds en and ru",
				key.Line, key.Value)
		}
		if err := value.Decode(target); err != nil {
			return err
		}
	}
	return nil
}

// Section holds either a flat list of items or the same items split into groups.
type Section struct {
	Items  []Item
	Groups map[string][]Item
}

func (s *Section) UnmarshalYAML(node *yaml.Node) error {
	switch node.Kind {
	case yaml.SequenceNode:
		return node.Decode(&s.Items)
	case yaml.MappingNode:
		// Writing a note straight under the section, without a group, is the typical
		// slip; the decoder's own message for it names Go types instead of saying so.
		for i := 0; i+1 < len(node.Content); i += 2 {
			key, value := node.Content[i], node.Content[i+1]
			if value.Kind != yaml.SequenceNode {
				return fmt.Errorf("line %d: group %q holds a list of items, and a section "+
					"without groups is a list itself", value.Line, key.Value)
			}
		}
		return node.Decode(&s.Groups)
	default:
		return fmt.Errorf("line %d: a section is either a list of items or a mapping of groups",
			node.Line)
	}
}

// Release is the notes of one version. Sections are separate fields, so an
// unknown key in the source is rejected by the decoder itself.
type Release struct {
	Version      string   `yaml:"version"`
	Date         string   `yaml:"date"`
	Highlights   *Section `yaml:"highlights"`
	Features     *Section `yaml:"features"`
	Improvements *Section `yaml:"improvements"`
	Fixes        *Section `yaml:"fixes"`
	Security     *Section `yaml:"security"`
	Breaking     *Section `yaml:"breaking"`
	Upgrade      *Section `yaml:"upgrade"`
	KnownIssues  *Section `yaml:"known_issues"`
	Docs         *Section `yaml:"docs"`
	Dependencies *Section `yaml:"dependencies"`
}

// Sections in the order the ADR prescribes, with the headings of both languages.
var sections = []struct {
	Key, En, Ru string
}{
	{"highlights", "Highlights", "Ключевые изменения"},
	{"features", "New features", "Новые возможности"},
	{"improvements", "Improvements", "Улучшения"},
	{"fixes", "Fixes", "Исправления"},
	{"security", "Security", "Безопасность"},
	{"breaking", "Breaking changes", "Несовместимые изменения"},
	{"upgrade", "Upgrade notes", "Рекомендации по обновлению"},
	{"known_issues", "Known issues", "Известные ограничения"},
	{"docs", "Docs", "Документация"},
	{"dependencies", "Dependencies", "Зависимости"},
}

// Groups a section can be split into. Few groups on purpose: a group of one
// item reads worse than no grouping, and a release of a big module brings 30+
// notes.
var groups = []struct {
	Key, En, Ru string
}{
	{"virtual-machines", "Virtual machines", "Виртуальные машины"},
	{"disks-and-images", "Disks and images", "Диски и образы"},
	{"network", "Network", "Сеть"},
	{"observability", "Monitoring", "Мониторинг"},
	{"cli", "CLI", "CLI"},
	{"module", "Module", "Модуль"},
}

var monthsRu = []string{
	"января", "февраля", "марта", "апреля", "мая", "июня",
	"июля", "августа", "сентября", "октября", "ноября", "декабря",
}

// bySection returns the sections of a release in the order they are rendered.
func (r *Release) bySection() []*Section {
	return []*Section{
		r.Highlights, r.Features, r.Improvements, r.Fixes, r.Security,
		r.Breaking, r.Upgrade, r.KnownIssues, r.Docs, r.Dependencies,
	}
}

// groupsInOrder returns the groups of a section that hold items, in the order
// they are rendered, plus the keys the section uses but the table does not know.
func (s *Section) groupsInOrder() (known, unknown []string) {
	seen := map[string]bool{}
	for _, group := range groups {
		if len(s.Groups[group.Key]) > 0 {
			known = append(known, group.Key)
		}
		seen[group.Key] = true
	}
	for key := range s.Groups {
		if !seen[key] {
			unknown = append(unknown, key)
		}
	}
	sort.Strings(unknown)
	return known, unknown
}

// allItems returns every item of a section, grouped or not.
func (s *Section) allItems() []Item {
	if s == nil {
		return nil
	}
	items := make([]Item, 0, len(s.Items))
	items = append(items, s.Items...)
	for _, group := range s.Groups {
		items = append(items, group...)
	}
	return items
}

func (s *Section) empty() bool {
	return s == nil || len(s.allItems()) == 0
}

func (r *Release) date() (time.Time, error) {
	return time.Parse("2006-01-02", r.Date)
}

// load reads the source, rejecting unknown section keys.
func load(path string) ([]Release, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	decoder := yaml.NewDecoder(bytes.NewReader(data))
	decoder.KnownFields(true)

	var releases []Release
	if err := decoder.Decode(&releases); err != nil {
		// The most common way to break the file is a note that carries ": " and is
		// left unquoted; the parser reports it as a mapping in the wrong place.
		if strings.Contains(err.Error(), "mapping values are not allowed") {
			return nil, fmt.Errorf("%s: %w (a note that contains \": \" has to be quoted "+
				"or written as a |- block)", path, err)
		}
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if len(releases) == 0 {
		return nil, fmt.Errorf("%s: no releases", path)
	}
	if err := rejectDanglingKeys(data, path); err != nil {
		return nil, err
	}
	trimItems(releases)
	return releases, nil
}

// rejectDanglingKeys catches a key left without a value: `fixes:` with nothing
// under it decodes to a nil pointer without ever reaching UnmarshalYAML, so the
// slip has to be caught on the raw tree.
func rejectDanglingKeys(data []byte, path string) error {
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil || len(doc.Content) == 0 {
		return nil // the decoder above has already accepted the document
	}
	for _, release := range doc.Content[0].Content {
		for i := 0; i+1 < len(release.Content); i += 2 {
			key, value := release.Content[i], release.Content[i+1]
			if value.Tag == "!!null" {
				return fmt.Errorf("%s: line %d: %q has no value, remove the key",
					path, key.Line, key.Value)
			}
		}
	}
	return nil
}

// trimItems drops the trailing newline a `|` block scalar keeps: a note is a
// paragraph, and the renderers put the line breaks around it themselves. Without
// this a note written with `|` instead of `|-` renders an empty list item after
// itself and reaches the Console with a trailing break.
func trimItems(releases []Release) {
	trim := func(items []Item) {
		for i := range items {
			items[i].En = strings.TrimRight(items[i].En, "\n")
			items[i].Ru = strings.TrimRight(items[i].Ru, "\n")
		}
	}
	for i := range releases {
		for _, section := range releases[i].bySection() {
			if section == nil {
				continue
			}
			trim(section.Items)
			for _, group := range section.Groups {
				trim(group)
			}
		}
	}
}

// newest is the release whose notes go into the release image. The order in the
// file is checked separately, so this does not rely on it.
func newest(releases []Release) *Release {
	latest := &releases[0]
	for i := range releases {
		if less(versionKey(latest.Version), versionKey(releases[i].Version)) {
			latest = &releases[i]
		}
	}
	return latest
}
