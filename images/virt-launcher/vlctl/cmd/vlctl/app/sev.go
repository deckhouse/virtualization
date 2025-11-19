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

package app

import (
	"fmt"

	"github.com/spf13/cobra"
)

func NewSevCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "sev",
		Short: "Get sev info",
		Args:  cobra.NoArgs,

		RunE: func(cmd *cobra.Command, _ []string) error {
			baseOpts := BaseOptionsFromCommand(cmd)
			return runSevCommand(baseOpts)
		},
	}
}

func runSevCommand(opts BaseOptions) error {
	if err := opts.Validate(); err != nil {
		return err
	}

	client, err := opts.Client()
	if err != nil {
		return fmt.Errorf("failed to create client: %w", err)
	}
	defer client.Close()

	info, err := client.GetSEVInfo()
	if err != nil {
		return fmt.Errorf("failed to get SEV info: %w", err)
	}

	return marshalAndPrintOutput(&opts, info)
}
