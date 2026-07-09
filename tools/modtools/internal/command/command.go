/*
Copyright 2024 Flant JSC

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

// Package command builds the modtools root command and wires the subcommands.
package command

import (
	"github.com/spf13/cobra"

	"github.com/deckhouse/virtualization/tools/modtools/internal/cmd/docchanges"
	"github.com/deckhouse/virtualization/tools/modtools/internal/cmd/dump"
	"github.com/deckhouse/virtualization/tools/modtools/internal/cmd/license"
	"github.com/deckhouse/virtualization/tools/modtools/internal/cmd/nocyrillic"
	"github.com/deckhouse/virtualization/tools/modtools/internal/diff"
)

func NewCommand(name string) *cobra.Command {
	var patchFile string

	root := &cobra.Command{
		Use:           name,
		Short:         "Repository maintenance checks for the virtualization module",
		Long:          "modtools bundles the repository maintenance checks: license headers, Cyrillic detection and documentation change validation.",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.PersistentFlags().StringVar(&patchFile, "file", "", "Patch file to read the diff from. Runs 'git diff' if not set.")

	load := diff.Loader(func() (*diff.DiffInfo, error) {
		return diff.Load(patchFile)
	})

	root.AddCommand(
		nocyrillic.NewCommand(load),
		docchanges.NewCommand(load),
		dump.NewCommand(load),
		license.NewCommand(),
	)
	return root
}
