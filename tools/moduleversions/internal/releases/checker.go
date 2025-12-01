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

package releases

import (
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"scaper/internal/version"

	"github.com/PuerkitoBio/goquery"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

const (
	httpTimeout        = 30 * time.Second
	expectedCellsCount = 6
)

var channelMap = map[string]int{
	// Channel column indices in the HTML table on releases.deckhouse.io
	"alpha":        1,
	"beta":         2,
	"early-access": 3,
	"stable":       4,
	"rock-solid":   5,
}

// VerifyVersionInEdition checks if the specified version exists for the given channel
// on the releases.deckhouse.io page for a specific edition.
func VerifyVersionInEdition(editionURL, channel, expectedVersion, moduleName string) (bool, error) {
	client := &http.Client{
		Timeout: httpTimeout,
	}

	resp, err := client.Get(editionURL)
	if err != nil {
		return false, fmt.Errorf("failed to fetch edition URL %s: %w", editionURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("unexpected status code %d for URL %s", resp.StatusCode, editionURL)
	}

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return false, fmt.Errorf("failed to parse HTML from %s: %w", editionURL, err)
	}

	var (
		index      int
		webVersion string
	)

	index, ok := channelMap[channel]
	if !ok {
		return false, fmt.Errorf("unknown channel: %s", channel)
	}
	if index < 0 || index >= expectedCellsCount {
		return false, fmt.Errorf("unknown channel: %s", channel)
	}

	doc.Find("tr").Each(func(i int, s *goquery.Selection) {
		if strings.Contains(s.Text(), moduleName) {
			cells := s.Find("td")
			if cells.Length() == expectedCellsCount {
				webVersion = strings.TrimSpace(cells.Eq(index).Text())
			}
		}
	})

	if webVersion == "" {
		return false, fmt.Errorf("version not found for module %s in channel %s", moduleName, channel)
	}

	normalizedWebVersion := version.NormalizeSemVer(webVersion)
	normalizedExpectedVersion := version.NormalizeSemVer(expectedVersion)

	if normalizedWebVersion != normalizedExpectedVersion {
		return false, fmt.Errorf("version mismatch: expected %s, got %s", normalizedExpectedVersion, normalizedWebVersion)
	}

	return true, nil
}

// VerifyVersionAcrossAllEditions checks if the specified version exists for the given channel
// across all supported editions on releases.deckhouse.io.
func VerifyVersionAcrossAllEditions(editionURLs []string, channel, expectedVersion, moduleName, baseURL string) (bool, *ModuleVersionInfo, error) {
	versionInfo := &ModuleVersionInfo{
		Module:   moduleName,
		Versions: []ChannelVersion{},
	}

	hasMatch := false
	var lastErr error

	for _, editionURL := range editionURLs {
		urlParts := strings.Split(editionURL, "/")
		if len(urlParts) < 4 {
			log.Printf("Warning: invalid URL format: %s", editionURL)
			continue
		}
		edition := urlParts[len(urlParts)-1]

		match, err := VerifyVersionInEdition(editionURL, channel, expectedVersion, moduleName)
		if err != nil {
			lastErr = err
			log.Printf("Error checking %-7s edition on channel %s: %v", edition, cases.Title(language.Und).String(channel), err)
			continue
		}

		if match {
			hasMatch = true
			versionInfo.Versions = append(versionInfo.Versions, ChannelVersion{
				Edition: edition,
				Channel: channel,
				Number:  expectedVersion,
			})
		}
	}

	if !hasMatch {
		return false, nil, fmt.Errorf("version %s not found in any edition for channel %s on %s: %w", expectedVersion, channel, baseURL, lastErr)
	}

	return true, versionInfo, nil
}
