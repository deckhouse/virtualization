package rewriter

import "strings"

type PrefixedNameRewriter struct {
	namesRenameIdx   map[string]string
	namesRestoreIdx  map[string]string
	prefixRenameIdx  map[string]string
	prefixRestoreIdx map[string]string
}

func NewPrefixedNameRewriter(replaceRules MetadataReplace) *PrefixedNameRewriter {
	return &PrefixedNameRewriter{
		namesRenameIdx:   indexRules(replaceRules.Names),
		namesRestoreIdx:  indexRulesReverse(replaceRules.Names),
		prefixRenameIdx:  indexRules(replaceRules.Prefixes),
		prefixRestoreIdx: indexRulesReverse(replaceRules.Prefixes),
	}
}

func (p *PrefixedNameRewriter) Rewrite(name string, action Action) string {
	switch action {
	case Rename:
		return p.rename(name)
	case Restore:
		return p.restore(name)
	}
	return name
}

func (p *PrefixedNameRewriter) RewriteSlice(names []string, action Action) []string {
	switch action {
	case Rename:
		return p.rewriteSlice(names, p.rename)
	case Restore:
		return p.rewriteSlice(names, p.restore)
	}
	return names
}

func (p *PrefixedNameRewriter) RewriteMap(names map[string]string, action Action) map[string]string {
	switch action {
	case Rename:
		return p.rewriteMap(names, p.rename)
	case Restore:
		return p.rewriteMap(names, p.restore)
	}
	return names
}

func (p *PrefixedNameRewriter) Rename(name string) string {
	return p.rename(name)
}

func (p *PrefixedNameRewriter) Restore(name string) string {
	return p.restore(name)
}

func (p *PrefixedNameRewriter) RenameSlice(names []string) []string {
	return p.rewriteSlice(names, p.rename)
}

func (p *PrefixedNameRewriter) RestoreSlice(names []string) []string {
	return p.rewriteSlice(names, p.restore)
}

func (p *PrefixedNameRewriter) RenameMap(names map[string]string) map[string]string {
	return p.rewriteMap(names, p.rename)
}

func (p *PrefixedNameRewriter) RestoreMap(names map[string]string) map[string]string {
	return p.rewriteMap(names, p.restore)
}

func (p *PrefixedNameRewriter) rewriteMap(names map[string]string, fn func(string) string) map[string]string {
	if names == nil {
		return nil
	}
	result := make(map[string]string)
	for name, value := range names {
		result[fn(name)] = value
	}
	return result
}

func (p *PrefixedNameRewriter) rewriteSlice(names []string, fn func(string) string) []string {
	if names == nil {
		return nil
	}
	result := make([]string, 0, len(names))
	for _, name := range names {
		result = append(result, fn(name))
	}
	return result
}

func (p *PrefixedNameRewriter) rename(name string) string {
	if renamed, ok := p.namesRenameIdx[name]; ok {
		return renamed
	}
	// No exact name, find prefix.
	prefix, remainder, found := strings.Cut(name, "/")
	if !found {
		return name
	}
	if renamedPrefix, ok := p.prefixRenameIdx[prefix]; ok {
		return renamedPrefix + "/" + remainder
	}
	return name
}

func (p *PrefixedNameRewriter) restore(name string) string {
	if restored, ok := p.namesRestoreIdx[name]; ok {
		return restored
	}
	// No exact name, find prefix.
	prefix, remainder, found := strings.Cut(name, "/")
	if !found {
		return name
	}
	if restoredPrefix, ok := p.prefixRestoreIdx[prefix]; ok {
		return restoredPrefix + "/" + remainder
	}
	return name
}

func indexRules(rules []MetadataReplaceRule) map[string]string {
	idx := make(map[string]string, len(rules))
	for _, rule := range rules {
		idx[rule.Original] = rule.Renamed
	}
	return idx
}

func indexRulesReverse(rules []MetadataReplaceRule) map[string]string {
	idx := make(map[string]string, len(rules))
	for _, rule := range rules {
		idx[rule.Renamed] = rule.Original
	}
	return idx
}
