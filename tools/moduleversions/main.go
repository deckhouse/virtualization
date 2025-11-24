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

package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"strings"
	"time"

	"scaper/internal/helper"

	"github.com/PuerkitoBio/goquery"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

const (
	// Channel column indices in the HTML table on releases.deckhouse.io
	channelAlphaIndex       = 1
	channelBetaIndex        = 2
	channelEarlyAccessIndex = 3
	channelStableIndex      = 4
	channelRockSolidIndex   = 5
	expectedCellsCount      = 6

	// HTTP client settings
	httpTimeout = 30 * time.Second
	retryDelay  = 10 * time.Second

	// Default values
	defaultModuleName       = "virtualization"
	defaultReleasesBaseURL  = "https://releases.deckhouse.io"
	defaultDocumentationURL = "https://deckhouse.ru/modules"
)

// Supported editions to check on releases.deckhouse.io
var supportedEditions = []string{"fe", "ee", "ce", "se-plus"}

// ChannelVersion represents a version found for a specific edition and channel
type ChannelVersion struct {
	Edition string
	Number  string
	Channel string
}

// ModuleVersionInfo contains information about versions found across editions
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

// verifyVersionInEdition checks if the specified version exists for the given channel
// on the releases.deckhouse.io page for a specific edition
func verifyVersionInEdition(editionURL, channel, expectedVersion, moduleName string) (bool, error) {
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

	switch channel {
	case "alpha":
		index = channelAlphaIndex
	case "beta":
		index = channelBetaIndex
	case "early-access":
		index = channelEarlyAccessIndex
	case "stable":
		index = channelStableIndex
	case "rock-solid":
		index = channelRockSolidIndex
	default:
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

	if webVersion != expectedVersion {
		return false, fmt.Errorf("version mismatch: expected %s, got %s", expectedVersion, webVersion)
	}

	return true, nil
}

// verifyVersionAcrossAllEditions checks if the specified version exists for the given channel
// across all supported editions on releases.deckhouse.io
func verifyVersionAcrossAllEditions(editionURLs []string, channel, expectedVersion, moduleName, baseURL string) (bool, *ModuleVersionInfo, error) {
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

		match, err := verifyVersionInEdition(editionURL, channel, expectedVersion, moduleName)
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

func main() {
	var count int

	channel := flag.String("channel", "", "release channel: alpha, beta, early-access, stable, rock-solid")
	version := flag.String("version", "", "module version to verify (e.g., 1.1.2 or v1.1.2)")
	moduleName := flag.String("module", defaultModuleName, "module name to check")
	baseURL := flag.String("base-url", defaultReleasesBaseURL, "base URL for releases.deckhouse.io")
	flag.IntVar(&count, "count", 10, "maximum number of retry attempts (default: 10)")

	// Custom usage function to provide better error messages
	flag.Usage = func() {
		fmt.Fprintf(flag.CommandLine.Output(), "Usage: %s [options]\n\n", os.Args[0])
		fmt.Fprintf(flag.CommandLine.Output(), "Options:\n")
		flag.PrintDefaults()
		fmt.Fprintf(flag.CommandLine.Output(), "\nExample:\n")
		fmt.Fprintf(flag.CommandLine.Output(), "  %s -channel alpha -version v1.1.3\n", os.Args[0])
	}

	flag.Parse()

	if *channel == "" || *version == "" {
		if *channel == "" {
			fmt.Fprintf(flag.CommandLine.Output(), "Error: -channel flag is required\n")
		}
		if *version == "" {
			fmt.Fprintf(flag.CommandLine.Output(), "Error: -version flag is required\n")
		}
		fmt.Fprintf(flag.CommandLine.Output(), "\n")
		flag.Usage()
		os.Exit(2)
	}

	normalizedChannel := helper.GetChannel(*channel)
	normalizedVersion := helper.GetSemVer(*version)

	// Build edition URLs for releases.deckhouse.io
	editionURLs := make([]string, 0, len(supportedEditions))
	for _, edition := range supportedEditions {
		editionURLs = append(editionURLs, *baseURL+"/"+edition)
	}

	// Verify version on releases.deckhouse.io across all editions
	fmt.Printf("Checking version %s on channel %s at %s...\n", normalizedVersion, normalizedChannel, *baseURL)

	var versionInfo *ModuleVersionInfo
	var err error
	releasesCheckPassed := false

	for attempt := 1; attempt <= count; attempt++ {
		releasesCheckPassed, versionInfo, err = verifyVersionAcrossAllEditions(editionURLs, normalizedChannel, normalizedVersion, *moduleName, *baseURL)
		if err != nil {
			if attempt < count {
				log.Printf("Attempt %d/%d failed: %v", attempt, count, err)
				fmt.Printf("Waiting %v before next attempt...\n", retryDelay)
				time.Sleep(retryDelay)
				continue
			}
			// Last attempt failed
			log.Fatalf("Version %s validation failed on %s after %d attempts: %v", normalizedVersion, *baseURL, count, err)
		}

		if releasesCheckPassed {
			fmt.Printf("Version %s is valid on channel %s\n", normalizedVersion, normalizedChannel)
			fmt.Println(versionInfo)
			break
		}
	}

	if !releasesCheckPassed {
		log.Fatalf("Version %s validation failed on %s after %d attempts: %v", normalizedVersion, *baseURL, count, err)
	}

	// Verify version on deckhouse.ru documentation site
	documentationURL := fmt.Sprintf("%s/%s/stable/", defaultDocumentationURL, *moduleName)
	fmt.Printf("\nChecking version %s on channel %s at %s...\n", normalizedVersion, normalizedChannel, documentationURL)

	err = helper.VerifyVersionOnDocumentationSite(documentationURL, normalizedChannel, normalizedVersion)
	if err != nil {
		log.Fatalf("Version %s validation failed on documentation site %s: %v", normalizedVersion, documentationURL, err)
	}

	fmt.Printf("Version %s is valid on channel %s at documentation site\n", normalizedVersion, normalizedChannel)
	fmt.Println("\nAll version checks passed successfully!")
}
