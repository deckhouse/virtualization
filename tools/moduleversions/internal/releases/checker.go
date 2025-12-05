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

	"moduleversions/internal/version"

	"github.com/PuerkitoBio/goquery"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

// ChannelVersion represents a version for a specific channel and edition.
type ChannelVersion struct {
	Edition string
	Number  string
	Channel string
}

// ModuleVersionInfo contains version information for a module across different editions.
type ModuleVersionInfo struct {
	Module   string
	Versions []ChannelVersion
}

func (v ModuleVersionInfo) String() string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("Module: %s\n", v.Module))
	for _, version := range v.Versions {
		b.WriteString(fmt.Sprintf("%-7s %s %s\n", version.Edition, version.Channel, version.Number))
	}
	return b.String()
}

const (
	httpTimeout        = 5 * time.Second
	expectedCellsCount = 6
	// minURLPartsCount is the minimum number of parts expected in a URL after splitting by "/"
	// For example: "https://releases.deckhouse.io/ee" -> ["https:", "", "releases.deckhouse.io", "ee"] = 4 parts
	minURLPartsCount = 4
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
		if len(urlParts) < minURLPartsCount {
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

const (
	defaultReleasesBaseURL = "https://releases.deckhouse.io"
	retryDelay             = 60 * time.Second
)

// Supported editions to check on releases.deckhouse.io
var supportedEditions = []string{"fe", "ee", "ce", "se-plus"}

// CheckVersionWithRetries checks version on releases.deckhouse.io with retry logic.
func CheckVersionWithRetries(channel, version, moduleName string, attempts int) error {
	editionURLs := make([]string, 0, len(supportedEditions))
	for _, edition := range supportedEditions {
		editionURLs = append(editionURLs, defaultReleasesBaseURL+"/"+edition)
	}

	fmt.Printf("Checking version %s on channel %s at %s...\n", version, channel, defaultReleasesBaseURL)

	for attempt := 1; attempt <= attempts; attempt++ {
		checkPassed, versionInfo, err := VerifyVersionAcrossAllEditions(editionURLs, channel, version, moduleName, defaultReleasesBaseURL)
		if err != nil {
			if attempt < attempts {
				log.Printf("Attempt %d/%d failed: %v", attempt, attempts, err)
				fmt.Printf("Waiting %v before next attempt...\n", retryDelay)
				time.Sleep(retryDelay)
				continue
			}
			// Last attempt failed
			log.Printf("Version %s validation failed on %s after %d attempts: %v", version, defaultReleasesBaseURL, attempts, err)
			return err
		}

		if checkPassed {
			fmt.Printf("Version %s is valid on channel %s\n", version, channel)
			fmt.Println(versionInfo)
			return nil
		}
	}

	// This should not happen if all attempts completed successfully
	// If we reach here, it means the loop completed without returning, which shouldn't happen
	return fmt.Errorf("version validation failed after %d attempts", attempts)
}
