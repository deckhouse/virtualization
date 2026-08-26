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
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	// The documentation renders links by path, so the source keeps them without a
	// domain; the Console shows the notes outside the documentation, where a path
	// leads nowhere.
	docBaseURL   = "https://deckhouse.io"
	docBaseURLRu = "https://deckhouse.ru"

	dateSpan = `<span style="opacity:0.6; font-style:italic; font-size:0.9em;">`
)

// renderMarkdown builds the release notes page of one language. The front matter
// is what docs-builder reads: the title of the page and its place in the sidebar.
// With absolute set, the links carry the site domain of the language: such a page
// is for pasting outside the documentation, where a path leads nowhere.
func renderMarkdown(releases []Release, lang string, absolute bool) (string, error) {
	title := "Release Notes"
	if lang == "ru" {
		title = "История изменений"
	}
	lines := []string{"---", fmt.Sprintf("title: %q", title), "weight: 70", "---", ""}

	for i := range releases {
		release := &releases[i]
		date, err := dateLine(release, lang)
		if err != nil {
			return "", err
		}
		lines = append(lines, "## "+release.Version, "", dateSpan, date, "</span>", "")

		for index, section := range release.bySection() {
			if section.empty() {
				continue
			}
			heading := sections[index].En
			if lang == "ru" {
				heading = sections[index].Ru
			}
			lines = append(lines, "### "+heading, "")

			if len(section.Groups) == 0 {
				lines = append(lines, itemLines(section.Items, lang)...)
				lines = append(lines, "")
				continue
			}
			known, unknown := section.groupsInOrder()
			if len(unknown) > 0 {
				return "", fmt.Errorf("%s/%s: unknown groups %v", release.Version,
					sections[index].Key, unknown)
			}
			for _, key := range known {
				lines = append(lines, "#### "+groupHeading(key, lang), "")
				lines = append(lines, itemLines(section.Groups[key], lang)...)
				lines = append(lines, "")
			}
		}
	}
	page := strings.TrimRight(strings.Join(lines, "\n"), "\n") + "\n"
	if absolute {
		base := docBaseURL
		if lang == "ru" {
			base = docBaseURLRu
		}
		page = absoluteLinks(page, base)
	}
	return page, nil
}

// itemLines renders items as list items, keeping the lines of a multi-line item
// as they are: their indentation is what makes nested lists work.
func itemLines(items []Item, lang string) []string {
	lines := make([]string, 0, len(items))
	for _, item := range items {
		text := item.En
		if lang == "ru" {
			text = item.Ru
		}
		first, rest, _ := strings.Cut(text, "\n")
		lines = append(lines, "- "+first)
		if rest != "" {
			lines = append(lines, strings.Split(rest, "\n")...)
		}
	}
	return lines
}

func dateLine(release *Release, lang string) (string, error) {
	date, err := release.date()
	if err != nil {
		return "", fmt.Errorf("%s: %w", release.Version, err)
	}
	if lang == "ru" {
		return fmt.Sprintf("Дата релиза: %d %s %d.", date.Day(),
			monthsRu[date.Month()-1], date.Year()), nil
	}
	return "Release date: " + date.Format("January 2, 2006") + ".", nil
}

func groupHeading(key, lang string) string {
	for _, group := range groups {
		if group.Key == key {
			if lang == "ru" {
				return group.Ru
			}
			return group.En
		}
	}
	return key
}

// renderChangelog builds the notes of one release the way the Console reads
// them: English only, links absolute.
func renderChangelog(release *Release) (string, error) {
	root := &yaml.Node{Kind: yaml.MappingNode}
	for index, section := range release.bySection() {
		if section.empty() {
			continue
		}
		root.Content = append(root.Content, scalar(sections[index].En))
		if len(section.Groups) == 0 {
			root.Content = append(root.Content, sequence(section.Items))
			continue
		}
		known, unknown := section.groupsInOrder()
		if len(unknown) > 0 {
			return "", fmt.Errorf("%s/%s: unknown groups %v", release.Version,
				sections[index].Key, unknown)
		}
		grouped := &yaml.Node{Kind: yaml.MappingNode}
		for _, key := range known {
			grouped.Content = append(grouped.Content, scalar(groupHeading(key, "en")),
				sequence(section.Groups[key]))
		}
		root.Content = append(root.Content, grouped)
	}

	var out bytes.Buffer
	encoder := yaml.NewEncoder(&out)
	encoder.SetIndent(2)
	if err := encoder.Encode(root); err != nil {
		return "", err
	}
	if err := encoder.Close(); err != nil {
		return "", err
	}
	return out.String(), nil
}

func scalar(value string) *yaml.Node {
	node := &yaml.Node{Kind: yaml.ScalarNode, Value: value}
	if strings.Contains(value, "\n") {
		node.Style = yaml.LiteralStyle
	}
	return node
}

func sequence(items []Item) *yaml.Node {
	node := &yaml.Node{Kind: yaml.SequenceNode}
	for _, item := range items {
		node.Content = append(node.Content, scalar(absoluteLinks(item.En, docBaseURL)))
	}
	return node
}

func absoluteLinks(text, base string) string {
	return strings.ReplaceAll(text, "](/", "]("+base+"/")
}
