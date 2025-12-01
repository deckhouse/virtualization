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
	"os"

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

// NewCommand creates and returns the root cobra command.
func NewCommand() *cobra.Command {
	rootCmd := &cobra.Command{
		Use:   "moduleversions",
		Short: "Verify module versions across releases.deckhouse.io and deckhouse.ru/modules",
		Long: `Verify module versions across releases.deckhouse.io and deckhouse.ru/modules.

This tool checks if a specified version exists for a given channel on both
releases.deckhouse.io (across all editions) and deckhouse.ru/modules documentation site.`,
		Example: `  moduleversions --channel alpha --version v1.1.3
  moduleversions --channel stable --version 1.1.2 --check-releases
  moduleversions --channel beta --version 1.1.1 --check-docs`,
		RunE:          Run,
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	rootCmd.Flags().StringP("channel", "c", "", "release channel: alpha, beta, early-access, stable, rock-solid (required)")
	rootCmd.Flags().StringP("version", "v", "", "module version to verify (e.g., 1.1.2 or v1.1.2) (required)")
	rootCmd.Flags().StringP("module", "m", defaultModuleName, "module name to check")
	rootCmd.Flags().Int("attempt", 1, "maximum number of retry attempts")
	rootCmd.Flags().Bool("check-releases", false, "check version on releases.deckhouse.io")
	rootCmd.Flags().Bool("check-docs", false, "check version on deckhouse.ru/modules")

	rootCmd.MarkFlagRequired("channel")
	rootCmd.MarkFlagRequired("version")

	return rootCmd
}
