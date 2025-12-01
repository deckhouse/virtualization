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
	"net/http"
	"strings"
	"time"

	"github.com/PuerkitoBio/goquery"
	"scaper/internal/version"
)

const httpTimeout = 30 * time.Second

// SiteInfo contains version information parsed from deckhouse.ru
type SiteInfo struct {
	Channels  map[string]string
	URL       string
	FetchTime time.Time
}

// VerifyVersion checks if the specified version exists for the given channel
// on the deckhouse.ru documentation site.
func VerifyVersion(docURL, expectedChannel, expectedVersion string) error {
	info := &SiteInfo{
		Channels:  map[string]string{},
		URL:       docURL,
		FetchTime: time.Now(),
	}

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
			info.Channels[channel] = ver
		}
	})

	// Verify the expected version for the expected channel
	foundVersion, exists := info.Channels[expectedChannel]
	if !exists {
		return fmt.Errorf("channel %s not found on documentation site %s", expectedChannel, docURL)
	}

	if foundVersion != expectedVersion {
		return fmt.Errorf("version mismatch on documentation site: expected %s for channel %s, got %s", expectedVersion, expectedChannel, foundVersion)
	}

	fmt.Printf("Found version %s for channel %s on documentation site\n", foundVersion, expectedChannel)
	return nil
}
