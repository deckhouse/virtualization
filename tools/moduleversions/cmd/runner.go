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

package cmd

import (
	"fmt"
	"log"
	"time"

	"scaper/internal/docs"
	"scaper/internal/releases"
	"scaper/internal/version"

	"github.com/spf13/cobra"
)

const (
	retryDelay = 60 * time.Second

	defaultReleasesBaseURL  = "https://releases.deckhouse.io"
	defaultDocumentationURL = "https://deckhouse.ru/modules"
)

// Supported editions to check on releases.deckhouse.io
var supportedEditions = []string{"fe", "ee", "ce", "se-plus"}

// Run executes the command logic.
func Run(cmd *cobra.Command, args []string) error {
	cfg := &Config{}

	var err error

	cfg.Channel, err = cmd.Flags().GetString("channel")
	if err != nil {
		return fmt.Errorf("failed to get channel flag: %w", err)
	}

	cfg.Version, err = cmd.Flags().GetString("version")
	if err != nil {
		return fmt.Errorf("failed to get version flag: %w", err)
	}

	cfg.ModuleName, err = cmd.Flags().GetString("module")
	if err != nil {
		return fmt.Errorf("failed to get module flag: %w", err)
	}

	cfg.Attempt, err = cmd.Flags().GetInt("attempt")
	if err != nil {
		return fmt.Errorf("failed to get attempt flag: %w", err)
	}
	if cfg.Attempt < 1 {
		return fmt.Errorf("attempt must be at least 1, got %d", cfg.Attempt)
	}

	cfg.CheckReleases, err = cmd.Flags().GetBool("check-releases")
	if err != nil {
		return fmt.Errorf("failed to get check-releases flag: %w", err)
	}

	cfg.CheckDocs, err = cmd.Flags().GetBool("check-docs")
	if err != nil {
		return fmt.Errorf("failed to get check-docs flag: %w", err)
	}

	if !cfg.CheckReleases && !cfg.CheckDocs {
		cfg.CheckReleases = true
		cfg.CheckDocs = true
	}

	normalizedChannel := version.NormalizeChannel(cfg.Channel)
	normalizedVersion := version.NormalizeSemVer(cfg.Version)

	var hasError bool

	// Verify version on releases.deckhouse.io across all editions
	if cfg.CheckReleases {
		editionURLs := make([]string, 0, len(supportedEditions))
		for _, edition := range supportedEditions {
			editionURLs = append(editionURLs, defaultReleasesBaseURL+"/"+edition)
		}

		fmt.Printf("Checking version %s on channel %s at %s...\n", normalizedVersion, normalizedChannel, defaultReleasesBaseURL)

		var versionInfo *releases.ModuleVersionInfo
		var err error
		releasesCheckPassed := false

		for attempt := 1; attempt <= cfg.Attempt; attempt++ {
			releasesCheckPassed, versionInfo, err = releases.VerifyVersionAcrossAllEditions(editionURLs, normalizedChannel, normalizedVersion, cfg.ModuleName, defaultReleasesBaseURL)
			if err != nil {
				if attempt < cfg.Attempt {
					log.Printf("Attempt %d/%d failed: %v", attempt, cfg.Attempt, err)
					fmt.Printf("Waiting %v before next attempt...\n", retryDelay)
					time.Sleep(retryDelay)
					continue
				}
				// Last attempt failed
				log.Printf("Version %s validation failed on %s after %d attempts: %v", normalizedVersion, defaultReleasesBaseURL, cfg.Attempt, err)
				hasError = true
				break
			}

			if releasesCheckPassed {
				fmt.Printf("Version %s is valid on channel %s\n", normalizedVersion, normalizedChannel)
				fmt.Println(versionInfo)
				break
			}
		}

		if !releasesCheckPassed && err != nil {
			log.Printf("Version %s validation failed on %s after %d attempts: %v", normalizedVersion, defaultReleasesBaseURL, cfg.Attempt, err)
			hasError = true
		}
	}

	// Verify version on deckhouse.ru documentation site
	if cfg.CheckDocs {
		documentationURL := fmt.Sprintf("%s/%s/stable/", defaultDocumentationURL, cfg.ModuleName)
		fmt.Printf("\nChecking version %s on channel %s at %s...\n", normalizedVersion, normalizedChannel, documentationURL)

		var err error
		docsCheckPassed := false

		for attempt := 1; attempt <= cfg.Attempt; attempt++ {
			err = docs.VerifyVersion(documentationURL, normalizedChannel, normalizedVersion)
			if err != nil {
				if attempt < cfg.Attempt {
					log.Printf("Attempt %d/%d failed: %v", attempt, cfg.Attempt, err)
					fmt.Printf("Waiting %v before next attempt...\n", retryDelay)
					time.Sleep(retryDelay)
					continue
				}
				// Last attempt failed
				log.Printf("Version %s validation failed on documentation site %s after %d attempts: %v", normalizedVersion, documentationURL, cfg.Attempt, err)
				hasError = true
				break
			}

			docsCheckPassed = true
			fmt.Printf("Version %s is valid on channel %s at documentation site\n", normalizedVersion, normalizedChannel)
			break
		}

		if !docsCheckPassed && err != nil {
			log.Printf("Version %s validation failed on documentation site %s after %d attempts: %v", normalizedVersion, documentationURL, cfg.Attempt, err)
			hasError = true
		}
	}

	if hasError {
		return fmt.Errorf("one or more version checks failed")
	}

	fmt.Println("\nAll version checks passed successfully!")
	return nil
}
