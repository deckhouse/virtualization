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
	"fmt"
	"log"
	"strings"
	"time"

	"scaper/internal/helper"

	"github.com/spf13/cobra"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
)

const (
	retryDelay = 60 * time.Second

	defaultModuleName       = "virtualization"
	defaultReleasesBaseURL  = "https://releases.deckhouse.io"
	defaultDocumentationURL = "https://deckhouse.ru/modules"
)

// Supported editions to check on releases.deckhouse.io
var supportedEditions = []string{"fe", "ee", "ce", "se-plus"}

type ChannelVersion struct {
	Edition string
	Number  string
	Channel string
}

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

		match, err := helper.VerifyVersionInEdition(editionURL, channel, expectedVersion, moduleName)
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

type config struct {
	channel       string
	version       string
	moduleName    string
	count         int
	checkReleases bool
	checkDocs     bool
}

func run(cmd *cobra.Command, args []string) error {
	cfg := &config{}

	cfg.channel, _ = cmd.Flags().GetString("channel")
	cfg.version, _ = cmd.Flags().GetString("version")
	cfg.moduleName, _ = cmd.Flags().GetString("module")
	cfg.count, _ = cmd.Flags().GetInt("count")
	cfg.checkReleases, _ = cmd.Flags().GetBool("check-releases")
	cfg.checkDocs, _ = cmd.Flags().GetBool("check-docs")

	if !cfg.checkReleases && !cfg.checkDocs {
		cfg.checkReleases = true
		cfg.checkDocs = true
	}

	normalizedChannel := helper.GetChannel(cfg.channel)
	normalizedVersion := helper.GetSemVer(cfg.version)

	var hasError bool

	// Verify version on releases.deckhouse.io across all editions
	if cfg.checkReleases {
		editionURLs := make([]string, 0, len(supportedEditions))
		for _, edition := range supportedEditions {
			editionURLs = append(editionURLs, defaultReleasesBaseURL+"/"+edition)
		}

		fmt.Printf("Checking version %s on channel %s at %s...\n", normalizedVersion, normalizedChannel, defaultReleasesBaseURL)

		var versionInfo *ModuleVersionInfo
		var err error
		releasesCheckPassed := false

		for attempt := 1; attempt <= cfg.count; attempt++ {
			releasesCheckPassed, versionInfo, err = verifyVersionAcrossAllEditions(editionURLs, normalizedChannel, normalizedVersion, cfg.moduleName, defaultReleasesBaseURL)
			if err != nil {
				if attempt < cfg.count {
					log.Printf("Attempt %d/%d failed: %v", attempt, cfg.count, err)
					fmt.Printf("Waiting %v before next attempt...\n", retryDelay)
					time.Sleep(retryDelay)
					continue
				}
				// Last attempt failed
				log.Printf("Version %s validation failed on %s after %d attempts: %v", normalizedVersion, defaultReleasesBaseURL, cfg.count, err)
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
			log.Printf("Version %s validation failed on %s after %d attempts: %v", normalizedVersion, defaultReleasesBaseURL, cfg.count, err)
			hasError = true
		}
	}

	// Verify version on deckhouse.ru documentation site
	if cfg.checkDocs {
		documentationURL := fmt.Sprintf("%s/%s/stable/", defaultDocumentationURL, cfg.moduleName)
		fmt.Printf("\nChecking version %s on channel %s at %s...\n", normalizedVersion, normalizedChannel, documentationURL)

		var err error
		docsCheckPassed := false

		for attempt := 1; attempt <= cfg.count; attempt++ {
			err = helper.VerifyVersionOnDocumentationSite(documentationURL, normalizedChannel, normalizedVersion)
			if err != nil {
				if attempt < cfg.count {
					log.Printf("Attempt %d/%d failed: %v", attempt, cfg.count, err)
					fmt.Printf("Waiting %v before next attempt...\n", retryDelay)
					time.Sleep(retryDelay)
					continue
				}
				// Last attempt failed
				log.Printf("Version %s validation failed on documentation site %s after %d attempts: %v", normalizedVersion, documentationURL, cfg.count, err)
				hasError = true
				break
			}

			docsCheckPassed = true
			fmt.Printf("Version %s is valid on channel %s at documentation site\n", normalizedVersion, normalizedChannel)
			break
		}

		if !docsCheckPassed && err != nil {
			log.Printf("Version %s validation failed on documentation site %s after %d attempts: %v", normalizedVersion, documentationURL, cfg.count, err)
			hasError = true
		}
	}

	if hasError {
		return fmt.Errorf("one or more version checks failed")
	}

	fmt.Println("\nAll version checks passed successfully!")
	return nil
}

func main() {
	rootCmd := &cobra.Command{
		Use:   "moduleversions",
		Short: "Verify module versions across releases.deckhouse.io and deckhouse.ru/modules",
		Long: `Verify module versions across releases.deckhouse.io and deckhouse.ru/modules.

This tool checks if a specified version exists for a given channel on both
releases.deckhouse.io (across all editions) and deckhouse.ru/modules documentation site.`,
		Example: `  moduleversions --channel alpha --version v1.1.3
  moduleversions --channel stable --version 1.1.2 --check-releases
  moduleversions --channel beta --version 1.1.1 --check-docs`,
		RunE:          run,
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	rootCmd.Flags().StringP("channel", "c", "", "release channel: alpha, beta, early-access, stable, rock-solid (required)")
	rootCmd.Flags().StringP("version", "v", "", "module version to verify (e.g., 1.1.2 or v1.1.2) (required)")
	rootCmd.Flags().StringP("module", "m", defaultModuleName, "module name to check")
	rootCmd.Flags().Int("count", 1, "maximum number of retry attempts")
	rootCmd.Flags().Bool("check-releases", false, "check version on releases.deckhouse.io")
	rootCmd.Flags().Bool("check-docs", false, "check version on deckhouse.ru/modules")

	rootCmd.MarkFlagRequired("channel")
	rootCmd.MarkFlagRequired("version")

	if err := rootCmd.Execute(); err != nil {
		log.Fatal(err)
	}
}
