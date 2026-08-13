/*
Copyright 2025 Flant JSC

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

package version

import (
	"regexp"
	"strings"
)

// versionWithTwoComponents matches versions like "1.74" that lack a patch component,
// but not "1.74.0" which already has three components.
var versionWithTwoComponents = regexp.MustCompile(`(\d+\.\d+)(?:\.\d+)?`)

// NormalizeSemVer removes the 'v' prefix from version string if present.
func NormalizeSemVer(version string) string {
	version = strings.TrimPrefix(version, "v")
	version = strings.TrimSpace(version)
	return version
}

// NormalizeSemVerRange ensures all version literals in a semver range expression
// have three components (Major.Minor.Patch). Versions like "1.74" become "1.74.0".
func NormalizeSemVerRange(rangeStr string) string {
	return versionWithTwoComponents.ReplaceAllStringFunc(rangeStr, func(match string) string {
		if strings.Count(match, ".") == 1 {
			return match + ".0"
		}
		return match
	})
}

// NormalizeChannel normalizes channel name by converting to lowercase and replacing spaces with hyphens.
func NormalizeChannel(channel string) string {
	channel = strings.ReplaceAll(strings.ToLower(channel), " ", "-")
	return channel
}
