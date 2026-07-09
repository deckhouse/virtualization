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

// Package dump implements the `dump` command that prints the parsed diff, for
// debugging the diff parser and the file/git input.
package dump

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/deckhouse/virtualization/tools/modtools/internal/diff"
)

func NewCommand(load diff.Loader) *cobra.Command {
	return &cobra.Command{
		Use:   "dump",
		Short: "Print the parsed diff (for debugging)",
		RunE: func(_ *cobra.Command, _ []string) error {
			info, err := load()
			if err != nil {
				return err
			}
			fmt.Printf("%s\n", info.Dump())
			return nil
		},
	}
}
