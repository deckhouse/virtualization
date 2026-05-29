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
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"

	"moduleversions/internal/version"
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
	fmt.Fprintf(&b, "Module: %s\n", v.Module)
	for _, version := range v.Versions {
		fmt.Fprintf(&b, "%-7s %s %s\n", version.Edition, version.Channel, version.Number)
	}
	return b.String()
}

const (
	httpTimeout        = 5 * time.Second
	expectedCellsCount = 6
)

var channelMap = map[string]int{
	// Channel column indices in the HTML table on releases.deckhouse.io/modules/{module}.
	// The first column contains the edition name.
	"alpha":        1,
	"beta":         2,
	"early-access": 3,
	"stable":       4,
	"rock-solid":   5,
}

// VerifyVersionOnModulePage checks if the specified version exists for the given channel
// on the releases.deckhouse.io module page.
func VerifyVersionOnModulePage(moduleURL, channel, expectedVersion, moduleName string) (bool, *ModuleVersionInfo, error) {
	client := &http.Client{
		Timeout: httpTimeout,
	}

	resp, err := client.Get(moduleURL)
	if err != nil {
		return false, nil, fmt.Errorf("failed to fetch module URL %s: %w", moduleURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return false, nil, fmt.Errorf("unexpected status code %d for URL %s", resp.StatusCode, moduleURL)
	}

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return false, nil, fmt.Errorf("failed to parse HTML from %s: %w", moduleURL, err)
	}

	index, ok := channelMap[channel]
	if !ok {
		return false, nil, fmt.Errorf("unknown channel: %s", channel)
	}
	if index < 0 || index >= expectedCellsCount {
		return false, nil, fmt.Errorf("unknown channel: %s", channel)
	}

	versionInfo := &ModuleVersionInfo{
		Module:   moduleName,
		Versions: []ChannelVersion{},
	}

	foundEdition := false
	normalizedExpectedVersion := version.NormalizeSemVer(expectedVersion)

	doc.Find("tr").Each(func(i int, s *goquery.Selection) {
		cells := s.Find("td")
		if cells.Length() != expectedCellsCount {
			return
		}

		edition := strings.TrimSpace(cells.Eq(0).Text())
		if edition == "" {
			return
		}

		foundEdition = true

		webVersion := strings.TrimSpace(cells.Eq(index).Text())
		if version.NormalizeSemVer(webVersion) != normalizedExpectedVersion {
			return
		}

		versionInfo.Versions = append(versionInfo.Versions, ChannelVersion{
			Edition: edition,
			Channel: channel,
			Number:  expectedVersion,
		})
	})

	if !foundEdition {
		return false, nil, fmt.Errorf("module %s was not found on %s", moduleName, moduleURL)
	}

	if len(versionInfo.Versions) == 0 {
		return false, nil, fmt.Errorf("version %s not found for module %s in channel %s on %s", expectedVersion, moduleName, channel, moduleURL)
	}

	return true, versionInfo, nil
}

const (
	defaultReleasesBaseURL = "https://releases.deckhouse.io"
	retryDelay             = 60 * time.Second
)

// CheckVersionWithRetries checks version on releases.deckhouse.io with retry logic.
func CheckVersionWithRetries(channel, version, moduleName string, attempts int) error {
	moduleURL := defaultReleasesBaseURL + "/modules/" + url.PathEscape(moduleName)

	fmt.Printf("Checking version %s on channel %s at %s...\n", version, channel, moduleURL)

	for attempt := 1; attempt <= attempts; attempt++ {
		checkPassed, versionInfo, err := VerifyVersionOnModulePage(moduleURL, channel, version, moduleName)
		if err != nil {
			if attempt < attempts {
				fmt.Printf("Attempt %d/%d failed: %v", attempt, attempts, err)
				fmt.Printf("Waiting %v before next attempt...\n", retryDelay)
				time.Sleep(retryDelay)
				continue
			}
			// Last attempt failed
			fmt.Printf("Version %s validation failed on %s after %d attempts: %v", version, moduleURL, attempts, err)
			return err
		}

		if checkPassed {
			fmt.Printf("Version %s is valid on channel %s\n", version, channel)
			fmt.Println(versionInfo)
			return nil
		}
	}

	return nil
}
