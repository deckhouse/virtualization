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

package mac

import (
	"fmt"
	"regexp"
	"strings"
)

func GenerateOUI(clusterUID string) string {
	if !validateUUID(clusterUID) {
		return ""
	}

	cleanUID := strings.ReplaceAll(clusterUID, "-", "")
	numBytes := len(cleanUID) / 2
	for i := 0; i < numBytes; i++ {
		switch cleanUID[2*i+1] {
		case '6', '2', 'a', 'e':
			start := 2 * i
			var oui string
			if start+6 <= len(cleanUID) {
				oui = cleanUID[start : start+6]
			} else {
				oui = cleanUID[start:]
				oui += cleanUID[:(6 - len(oui))]
			}
			return formatOUI(oui)
		}
	}

	oui := cleanUID[:6]
	oui = oui[:1] + "2" + oui[2:]

	return formatOUI(oui)
}

func formatOUI(prefix string) string {
	prefix = strings.TrimSpace(prefix)

	re := regexp.MustCompile(`(?i)([0-9A-Fa-f]{2})`)
	matches := re.FindAllString(prefix, -1)

	return fmt.Sprintf("%s:%s:%s", matches[0], matches[1], matches[2])
}

func validateUUID(uid string) bool {
	matched, _ := regexp.MatchString("^[a-fA-F0-9]{8}-[a-fA-F0-9]{4}-[a-fA-F0-9]{4}-[a-fA-F0-9]{4}-[a-fA-F0-9]{12}$", uid)
	return matched
}
