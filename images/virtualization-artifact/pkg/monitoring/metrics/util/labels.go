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

package util

import (
	"fmt"
	"regexp"
	"strings"
)

var (
	invalidLabelCharRE = regexp.MustCompile(`[^a-zA-Z0-9_]`)
	matchAllCap        = regexp.MustCompile("([a-z0-9])([A-Z])")
)

type SkipLabel func(key, value string) bool

func WrapPrometheusLabels(labels map[string]string, prefix string, skip SkipLabel) map[string]string {
	wrapLabels := make(map[string]string, len(labels))
	conflicts := make(map[string]int, len(labels))

	for k, v := range labels {
		if skip != nil && skip(k, v) {
			continue
		}
		labelKey := labelName(prefix, k)
		if conflictCount, ok := conflicts[labelKey]; ok {
			if conflictCount == 1 {
				// this is the first conflict for the label,
				// so we have to go back and rename the initial label that we've already added

				value := wrapLabels[labelKey]
				delete(wrapLabels, labelKey)
				wrapLabels[labelConflictSuffix(labelKey, conflictCount)] = value
			}
			conflicts[labelKey]++
			labelKey = labelConflictSuffix(labelKey, conflicts[labelKey])
		} else {
			conflicts[labelKey] = 1
		}

		wrapLabels[labelKey] = v
	}
	return wrapLabels
}

func labelName(prefix, labelName string) string {
	return prefix + "_" + lintLabelName(SanitizeLabelName(labelName))
}

func SanitizeLabelName(s string) string {
	return invalidLabelCharRE.ReplaceAllString(s, "_")
}

func lintLabelName(s string) string {
	return toSnakeCase(s)
}

func toSnakeCase(s string) string {
	snake := matchAllCap.ReplaceAllString(s, "${1}_${2}")
	return strings.ToLower(snake)
}

func labelConflictSuffix(label string, count int) string {
	return fmt.Sprintf("%s_conflict%d", label, count)
}
