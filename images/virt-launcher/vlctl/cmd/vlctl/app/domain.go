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

func NewDomainCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "domain",
		Short: "Get domain specification",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			baseOpts := BaseOptionsFromCommand(cmd)
			return runDomainCommand(baseOpts)
		},
	}

	cmd.AddCommand(NewDomainStatsCommand())
	return cmd
}

func runDomainCommand(opts BaseOptions) error {
	if err := opts.Validate(); err != nil {
		return err
	}

	client, err := opts.Client()
	if err != nil {
		return fmt.Errorf("failed to create client: %w", err)
	}
	defer client.Close()

	domain, exist, err := client.GetDomain()
	if err != nil {
		return fmt.Errorf("failed to get domain: %w", err)
	}

	if !exist {
		return fmt.Errorf("domain does not exist")
	}

	return marshalAndPrintOutput(&opts, domain.Spec)
}

func NewDomainStatsCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "stats",
		Short: "Get domain statistics",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			baseOpts := BaseOptionsFromCommand(cmd)
			return runDomainStatsCommand(baseOpts)
		},
	}

}

func runDomainStatsCommand(opts BaseOptions) error {
	if err := opts.Validate(); err != nil {
		return err
	}

	client, err := opts.Client()
	if err != nil {
		return fmt.Errorf("failed to create client: %w", err)
	}
	defer client.Close()

	stats, exists, err := client.GetDomainStats()
	if err != nil {
		return fmt.Errorf("failed to get domain stats: %w", err)
	}
	if !exists {
		return fmt.Errorf("domain stats does not exist")
	}

	return marshalAndPrintOutput(&opts, stats)
}
