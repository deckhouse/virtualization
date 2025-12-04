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
	"os"

	"moduleversions/internal/docs"
	"moduleversions/internal/releases"
	"moduleversions/internal/version"

	"github.com/spf13/cobra"
)

const defaultModuleName = "virtualization"

// Execute runs the root command.
func Execute() {
	rootCmd := NewCommand()
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}

// Config holds the configuration for the command.
type Config struct {
	Channel       string
	Version       string
	ModuleName    string
	Attempt       int
	CheckReleases bool
	CheckDocs     bool
}

var cfg = &Config{}

// NewCommand creates and returns the root cobra command.
func NewCommand() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "moduleversions",
		Short: "Verify module versions across releases.deckhouse.io and deckhouse.ru/modules/virtualization/[channel]/",
		Long: `Verify module versions across releases.deckhouse.io and deckhouse.ru/modules/virtualization/[channel]/.

This tool checks if a specified version exists for a given channel on both
releases.deckhouse.io (across all editions) and deckhouse.ru/modules/virtualization/[channel]/ documentation site.`,
		Example: `  moduleversions --channel alpha --version v1.1.3
  moduleversions --channel stable --version 1.1.2 --check-releases
  moduleversions --channel beta --version 1.1.1 --check-docs`,
		RunE:          Run,
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	rootCmd.Flags().StringVarP(&cfg.Channel, "channel", "c", "", "release channel: alpha, beta, early-access, stable, rock-solid (required)")
	rootCmd.Flags().StringVarP(&cfg.Version, "version", "v", "", "module version to verify (e.g., 1.1.2 or v1.1.2) (required)")
	rootCmd.Flags().StringVarP(&cfg.ModuleName, "module", "m", defaultModuleName, "module name to check")
	rootCmd.Flags().IntVarP(&cfg.Attempt, "attempt", "a", 1, "maximum number of retry attempts")
	rootCmd.Flags().BoolVarP(&cfg.CheckReleases, "check-releases", "r", false, "check version on releases.deckhouse.io")
	rootCmd.Flags().BoolVarP(&cfg.CheckDocs, "check-docs", "d", false, "check version on deckhouse.ru/modules/virtualization/[channel]/")

	rootCmd.MarkFlagRequired("channel")
	rootCmd.MarkFlagRequired("version")

	return rootCmd
}

// Run executes the command logic.
func Run(cmd *cobra.Command, args []string) error {
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
