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
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/ast"
	"github.com/yuin/goldmark/text"
	"gopkg.in/yaml.v3"
)

// changesLanguage is the info string that turns a fenced code block of a merge
// request description into changelog entries.
const changesLanguage = "changes"

// Entry is one line of the changelog: what changed, in which part of the
// module, and how much the update asks of the reader.
type Entry struct {
	Section     string
	Type        string
	Summary     string
	Impact      string
	ImpactLevel string

	// Where the entry came from. collect fills these in; check works on an
	// open merge request and needs neither.
	MRIID int
	MRURL string
}

// rawEntry is one YAML document of a ```changes block.
//
// The v1 field names are still accepted, as deckhouse/changelog-action@v2.6.0
// accepts them: module for section, description for summary, note for impact.
type rawEntry struct {
	Section     string `yaml:"section"`
	Module      string `yaml:"module"`
	Type        string `yaml:"type"`
	Summary     string `yaml:"summary"`
	Description string `yaml:"description"`
	Impact      string `yaml:"impact"`
	Note        string `yaml:"note"`
	ImpactLevel string `yaml:"impact_level"`
}

// Blocks returns the body of every ```changes fenced block of a merge request
// description, in the order they appear.
//
// The blocks are found with a Markdown parser rather than with a regular
// expression, because a regular expression cannot tell a code block from
// something that merely looks like one. The example the merge request template
// ships is indented and lives inside an HTML comment, and both traits defeated
// the expressions this tool replaces: the collector read the example as a real
// entry and lost the entry the author had written under it — six entries of
// CHANGELOG-v1.11.0 are the template's text and not their authors'.
//
// A Markdown parser also agrees with upstream deckhouse/changelog-action, which
// reads the description with the marked lexer. CommonMark is enough here: the
// GFM extensions change inline markup and tables, not what a fenced code block
// or an HTML block is.
func Blocks(description string) []string {
	source := []byte(description)
	document := goldmark.New().Parser().Parse(text.NewReader(source))

	var blocks []string
	_ = ast.Walk(document, func(node ast.Node, entering bool) (ast.WalkStatus, error) {
		if !entering {
			return ast.WalkContinue, nil
		}
		fenced, ok := node.(*ast.FencedCodeBlock)
		if !ok || string(fenced.Language(source)) != changesLanguage {
			return ast.WalkContinue, nil
		}
		var body strings.Builder
		for i := 0; i < fenced.Lines().Len(); i++ {
			line := fenced.Lines().At(i)
			body.Write(line.Value(source))
		}
		blocks = append(blocks, body.String())
		return ast.WalkContinue, nil
	})
	return blocks
}

// ParsedBlock is one ```changes block of a merge request description: the
// entries it holds, or the reason YAML could not read it.
type ParsedBlock struct {
	// Index is the position of the block in the description, counted from one,
	// so that a message can point at the block the author has to fix.
	Index   int
	Entries []Entry
	Err     error
}

// ParseDescription reads every entry of a merge request description.
//
// The blocks come back in the order they were written, each with its own
// result: a block YAML cannot read carries the reason and no entries, and the
// blocks around it are still parsed. One broken block hides neither the others
// nor itself.
func ParseDescription(description string) []ParsedBlock {
	blocks := Blocks(description)
	parsed := make([]ParsedBlock, 0, len(blocks))
	for index, block := range blocks {
		entries, err := ParseBlock(block)
		parsed = append(parsed, ParsedBlock{Index: index + 1, Entries: entries, Err: err})
	}
	return parsed
}

// ParseBlock turns the body of one ```changes block into entries.
//
// A block holds one entry per YAML document, so `---` lines separate several
// entries, and a comma-separated section writes the same entry into each
// section it names. Both are how deckhouse/changelog-action@v2.6.0 reads a
// block.
//
// A block that cannot be read yields no entries at all, not the ones before the
// line that broke it: half of a block is not something to publish.
func ParseBlock(block string) ([]Entry, error) {
	var entries []Entry
	decoder := yaml.NewDecoder(strings.NewReader(block))
	for index := 1; ; index++ {
		var document yaml.Node
		err := decoder.Decode(&document)
		if errors.Is(err, io.EOF) {
			return entries, nil
		}
		if err != nil {
			return nil, documentError(index, err)
		}
		parsed, err := parseDocument(&document)
		if err != nil {
			return nil, documentError(index, err)
		}
		entries = append(entries, parsed...)
	}
}

func parseDocument(document *yaml.Node) ([]Entry, error) {
	node := document
	if node.Kind == yaml.DocumentNode {
		if len(node.Content) == 0 {
			return nil, nil
		}
		node = node.Content[0]
	}
	// A `---` line with nothing under it leaves an empty document behind. It
	// carries no entry and is not an error: a block may end with a separator.
	if node.Kind == yaml.ScalarNode && node.Tag == "!!null" {
		return nil, nil
	}
	if node.Kind != yaml.MappingNode {
		return nil, errors.New("an entry must be written as `key: value` lines")
	}

	var raw rawEntry
	if err := node.Decode(&raw); err != nil {
		return nil, cleanTypeName(err)
	}

	// The v1 name wins when a block carries both, as it does upstream. No block
	// is written with both in practice; the order only has to be fixed.
	section := firstNonEmpty(raw.Module, raw.Section)
	base := Entry{
		Type:        strings.TrimSpace(raw.Type),
		Summary:     strings.TrimSpace(firstNonEmpty(raw.Description, raw.Summary)),
		Impact:      strings.TrimSpace(firstNonEmpty(raw.Note, raw.Impact)),
		ImpactLevel: strings.TrimSpace(raw.ImpactLevel),
	}

	var entries []Entry
	for _, name := range strings.Split(section, ",") {
		entry := base
		entry.Section = normalizeSection(name)
		entries = append(entries, entry)
	}
	return entries, nil
}

// documentError numbers the document only when a block holds several: "block
// #1: ..." reads better than "block #1, entry #1: ..." for the common block
// that holds a single entry.
func documentError(index int, err error) error {
	if index == 1 {
		return err
	}
	return fmt.Errorf("entry #%d: %w", index, err)
}

// cleanTypeName keeps the Go type of the decoder out of a message an author of
// a merge request reads.
func cleanTypeName(err error) error {
	message := strings.ReplaceAll(err.Error(), "main.rawEntry", "a changelog entry")
	return errors.New(message)
}

// normalizeSection drops the forced impact level a section name used to carry.
//
// The list of allowed sections is written as `name:forced_level` (`ci:low`),
// and blocks written before that format was settled name the section the same
// way. The level of a section is read from the list, never from a block, so the
// suffix means nothing here — but a block that carries it still names a real
// section, and both run types must resolve it to the same one.
func normalizeSection(name string) string {
	name, _, _ = strings.Cut(strings.TrimSpace(name), ":")
	return strings.TrimSpace(name)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
