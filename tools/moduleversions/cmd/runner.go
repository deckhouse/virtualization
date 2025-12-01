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

	"scaper/internal/docs"
	"scaper/internal/releases"
	"scaper/internal/version"

	"github.com/spf13/cobra"
)

// Config holds the configuration for the command.
type Config struct {
	Channel       string
	Version       string
	ModuleName    string
	Attempt       int
	CheckReleases bool
	CheckDocs     bool
}

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
		err := releases.CheckVersionWithRetries(normalizedChannel, normalizedVersion, cfg.ModuleName, cfg.Attempt)
		if err != nil {
			hasError = true
		}
	}

	// Verify version on deckhouse.ru documentation site
	if cfg.CheckDocs {
		err := docs.CheckVersionWithRetries(normalizedChannel, normalizedVersion, cfg.ModuleName, cfg.Attempt)
		if err != nil {
			hasError = true
		}
	}

	if hasError {
		return fmt.Errorf("one or more version checks failed")
	}

	fmt.Println("\nAll version checks passed successfully!")
	return nil
}
