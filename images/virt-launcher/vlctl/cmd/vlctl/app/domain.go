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
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

const xmlDir = "/var/run/libvirt/qemu"

func NewDomainCommand() *cobra.Command {
	var fromFile bool

	cmd := &cobra.Command{
		Use:   "domain",
		Short: "Get domain specification",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			baseOpts := BaseOptionsFromCommand(cmd)
			return runDomainCommand(baseOpts, fromFile)
		},
	}

	cmd.Flags().BoolVarP(&fromFile, "from-file", "f", false, "Read domain specification from file")
	cmd.AddCommand(NewDomainStatsCommand())

	return cmd
}

func runDomainCommand(opts BaseOptions, fromFile bool) error {
	if fromFile {
		if opts.Output != outputXml {
			return fmt.Errorf("output format must be xml when reading from file")
		}
		entries, err := os.ReadDir(xmlDir)
		if err != nil {
			return fmt.Errorf("failed to read domain xml dir: %w", err)
		}
		for _, entry := range entries {
			if strings.HasSuffix(entry.Name(), ".xml") {
				b, err := os.ReadFile(filepath.Join(xmlDir, entry.Name()))
				if err != nil {
					return fmt.Errorf("failed to read domain xml file: %w", err)
				}
				_, err = os.Stdout.Write(append(b, '\n'))
				return err
			}
		}
		return fmt.Errorf("xml domain not found")
	}

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
