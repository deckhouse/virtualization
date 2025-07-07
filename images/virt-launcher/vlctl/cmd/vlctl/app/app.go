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

import "github.com/spf13/cobra"

const vlctlLong = `
      .__          __  .__   
___  _|  |   _____/  |_|  |  
\  \/ /  | _/ ___\   __\  |  
 \   /|  |_\  \___|  | |  |__
  \_/ |____/\___  >__| |____/
                \/           

vlctl is a tool for gathering information from virtual machines running under the
Kubernetes environment using the virt-launcher. It allows you to query
virtual machine stats, guest OS details, user information, file systems,
and more. The tool is designed to interact with virt-launcher to provide
detailed insights into virtual machine states without altering their operation.

Use the vlctl command to retrieve essential data from virtual machines.
`

func NewVlctlCommand() *cobra.Command {
	baseOpts := BaseOptions{}

	cmd := &cobra.Command{
		Use:           "vlctl",
		Short:         "vlctl - A tool to retrieve information from virtual machines managed by virt-launcher",
		Long:          vlctlLong,
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			cmd.SetContext(WithBaseOptions(cmd.Context(), baseOpts))
			return nil
		},
	}

	cmd.AddCommand(
		NewDomainCommand(),
		NewGuestCommand(),
		NewPingCommand(),
		NewQemuCommand(),
		NewSevCommand(),
	)

	flagset := cmd.PersistentFlags()
	baseOpts.AddFlags(flagset)

	return cmd
}
