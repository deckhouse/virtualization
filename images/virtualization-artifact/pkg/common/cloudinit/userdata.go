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

package cloudinit

import (
	"bytes"
	"fmt"
	"regexp"
	"strings"

	"sigs.k8s.io/yaml"
)

// The headers cloud-init recognizes at the start of a user data payload. Matching
// is case-insensitive, and the longest header wins, so #cloud-config-archive is
// never taken for a #cloud-config.
//
// See https://cloudinit.readthedocs.io/en/latest/explanation/format.html
const (
	// headerJinja renders the payload as a Jinja template first, and only then
	// looks for the header of the result.
	headerJinja = "## template: jinja"
	// headerCloudConfig marks a YAML mapping of cloud-config module settings.
	headerCloudConfig = "#cloud-config"
	// headerCloudConfigArchive marks a YAML list of payloads, each carrying its
	// own type.
	headerCloudConfigArchive = "#cloud-config-archive"
	// headerCloudConfigJSONP marks a list of RFC 6902 style operations patched
	// into the assembled cloud-config.
	headerCloudConfigJSONP = "#cloud-config-jsonp"
	// headerScript marks a script the guest runs once, on first boot.
	headerScript = "#!"
	// headerInclude marks a list of URLs to fetch further user data from.
	headerInclude = "#include"
	// headerIncludeOnce is headerInclude fetched only once, on first boot.
	headerIncludeOnce = "#include-once"
	// headerBoothook marks a script run very early, on every boot.
	headerBoothook = "#cloud-boothook"
	// headerPartHandler marks Python code teaching cloud-init a new part type.
	headerPartHandler = "#part-handler"
	// headerUpstartJob marks an Upstart job, dropped by recent cloud-init but
	// still recognized here.
	headerUpstartJob = "#upstart-job"
	// headerMIMEVersion makes cloud-init read the payload as a MIME multi part
	// archive. It does not have to be on the first line.
	headerMIMEVersion = "MIME-Version:"
	// headerContentType opens a MIME archive that states its type before its
	// version.
	headerContentType = "Content-Type:"
)

// knownHeaders is every recognized header, in the order they are listed in the
// message when a payload matches none of them.
var knownHeaders = []string{
	headerCloudConfig,
	headerCloudConfigArchive,
	headerCloudConfigJSONP,
	headerScript,
	headerInclude,
	headerIncludeOnce,
	headerBoothook,
	headerPartHandler,
	headerUpstartJob,
	headerJinja,
	headerMIMEVersion,
	headerContentType,
}

// matchOrder is knownHeaders arranged so that no header is shadowed by a shorter
// header it begins with.
var matchOrder = []string{
	headerJinja,
	headerCloudConfigArchive,
	headerCloudConfigJSONP,
	headerCloudConfig,
	headerBoothook,
	headerPartHandler,
	headerIncludeOnce,
	headerInclude,
	headerUpstartJob,
	headerMIMEVersion,
	headerContentType,
	headerScript,
}

// templateHeaderRe matches a first line meant to be headerJinja but spelled some
// other way, such as ##template: jinja or ## template:jinja. cloud-init picks the
// format of user data by comparing its start against the headers literally, so
// only the exact spelling makes it a template; the flexible ##\s*template: regexp
// in cloud-init's templater runs later, on a payload already recognized as one.
// Such a payload is ignored just like any other headerless one, but naming the
// header the author was reaching for beats listing every format again.
var templateHeaderRe = regexp.MustCompile(`(?i)^##[ \t]*template[ \t]*:[ \t]*[a-z0-9._-]+`)

// gzipMagic are the first bytes of a gzip stream. cloud-init accepts compressed
// user data.
var gzipMagic = []byte{0x1f, 0x8b}

// mimeSearchLimit is how far into the payload cloud-init looks for the
// MIME-Version header before giving up on reading it as a MIME archive.
const mimeSearchLimit = 4096

// maxQuotedLen bounds how much of the payload is echoed back in a message: user
// data can be kilobytes long, and messages end up in conditions and events.
const maxQuotedLen = 48

// ValidateUserData reports what looks wrong with a cloud-init user data payload.
// Every finding is a warning; an empty result means nothing was found.
func ValidateUserData(data []byte) []string {
	// Nothing to inspect in a compressed payload without decompressing it.
	if bytes.HasPrefix(data, gzipMagic) {
		return nil
	}

	text := strings.TrimLeft(string(data), "\ufeff \t\r\n")
	if text == "" {
		return []string{"user data is empty"}
	}

	lower := strings.ToLower(text)

	// The MIME version header may appear anywhere in the first few kilobytes,
	// not only at the very start of the payload.
	if strings.Contains(head(lower, mimeSearchLimit), strings.ToLower(headerMIMEVersion)) {
		return nil
	}

	for _, header := range matchOrder {
		if !strings.HasPrefix(lower, strings.ToLower(header)) {
			continue
		}

		switch header {
		case headerCloudConfig:
			return validateCloudConfig(text)
		case headerCloudConfigArchive, headerCloudConfigJSONP:
			return validateArchive(text, header)
		default:
			// A Jinja template is not valid YAML until it is rendered, and the
			// remaining formats are not YAML at all.
			return nil
		}
	}

	line := firstLine(text)

	if templateHeaderRe.MatchString(line) {
		return []string{fmt.Sprintf(
			"user data starts with %s, and cloud-init reads a template header only when it is spelled exactly %q, so it will ignore the payload",
			quote(line), headerJinja,
		)}
	}

	return []string{fmt.Sprintf(
		"user data starts with %s instead of a cloud-init header, so cloud-init will ignore it; expected one of: %s",
		quote(line), strings.Join(knownHeaders, ", "),
	)}
}

// validateCloudConfig checks that the document is YAML describing a mapping,
// which is the only shape cloud-init accepts for a cloud-config.
func validateCloudConfig(text string) []string {
	var config map[string]any
	if err := yaml.Unmarshal([]byte(text), &config); err != nil {
		return []string{fmt.Sprintf(
			"user data is not a valid cloud-config, so cloud-init will refuse it: %s",
			firstLine(err.Error()),
		)}
	}
	return nil
}

// validateArchive checks the shape of the two list-valued formats. What the
// entries carry is left alone: an entry may hold anything, including a payload for
// a part handler shipped in the same archive.
func validateArchive(text, header string) []string {
	var entries []any
	if err := yaml.Unmarshal([]byte(text), &entries); err != nil {
		return []string{fmt.Sprintf(
			"user data declares %s but is not a list of entries, so cloud-init will refuse it: %s",
			header, firstLine(err.Error()),
		)}
	}
	return nil
}

func head(s string, n int) string {
	if len(s) > n {
		return s[:n]
	}
	return s
}

func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	return strings.TrimRight(s, "\r")
}

func quote(s string) string {
	if len(s) > maxQuotedLen {
		return fmt.Sprintf("%q...", s[:maxQuotedLen])
	}
	return fmt.Sprintf("%q", s)
}
