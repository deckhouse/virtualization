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

package docs

import (
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"moduleversions/internal/version"

	"github.com/PuerkitoBio/goquery"
)

const httpTimeout = 5 * time.Second

// VerifyVersion checks if the specified version exists for the given channel
// on the deckhouse.ru documentation site.
func VerifyVersion(docURL, expectedChannel, expectedVersion string) error {
	channels := make(map[string]string)

	client := &http.Client{
		Timeout: httpTimeout,
	}

	resp, err := client.Get(docURL)
	if err != nil {
		return fmt.Errorf("failed to fetch documentation URL %s: %w", docURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code %d for URL %s", resp.StatusCode, docURL)
	}

	doc, err := goquery.NewDocumentFromReader(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to parse HTML from %s: %w", docURL, err)
	}

	// Parse channel-version pairs from the documentation site
	doc.Find(".submenu-item").Each(func(i int, s *goquery.Selection) {
		channel := s.Find(".submenu-item-channel").Text()
		channel = strings.TrimSpace(channel)
		channel = version.NormalizeChannel(channel)

		ver := s.Find(".submenu-item-release").Text()
		ver = version.NormalizeSemVer(ver)

		if channel != "" && ver != "" {
			channels[channel] = ver
		}
	})

	// Verify the expected version for the expected channel
	foundVersion, exists := channels[expectedChannel]
	if !exists {
		return fmt.Errorf("channel %s not found on documentation site %s", expectedChannel, docURL)
	}

	if foundVersion != expectedVersion {
		return fmt.Errorf("version mismatch on documentation site: expected %s for channel %s, got %s", expectedVersion, expectedChannel, foundVersion)
	}

	fmt.Printf("Found version %s for channel %s on documentation site\n", foundVersion, expectedChannel)
	return nil
}

const (
	defaultDocumentationURL = "https://deckhouse.ru/modules"
	retryDelay              = 60 * time.Second
)

// CheckVersionWithRetries checks version on documentation site with retry logic.
func CheckVersionWithRetries(channel, version, moduleName string, attempts int) error {
	documentationURL := fmt.Sprintf("%s/%s/%s/", defaultDocumentationURL, moduleName, channel)
	fmt.Printf("\nChecking version %s on channel %s at %s...\n", version, channel, documentationURL)

	var err error

	for attempt := 1; attempt <= attempts; attempt++ {
		err = VerifyVersion(documentationURL, channel, version)
		if err != nil {
			if attempt < attempts {
				log.Printf("Attempt %d/%d failed: %v", attempt, attempts, err)
				fmt.Printf("Waiting %v before next attempt...\n", retryDelay)
				time.Sleep(retryDelay)
				continue
			}
			// Last attempt failed
			log.Printf("Version %s validation failed on documentation site %s after %d attempts: %v", version, documentationURL, attempts, err)
			return err
		}

		fmt.Printf("Version %s is valid on channel %s at documentation site\n", version, channel)
		return nil
	}

	// This should not happen, but handle it just in case
	if err != nil {
		log.Printf("Version %s validation failed on documentation site %s after %d attempts: %v", version, documentationURL, attempts, err)
		return fmt.Errorf("version validation failed after %d attempts: %w", attempts, err)
	}

	return nil
}
