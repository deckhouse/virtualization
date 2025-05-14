/*
Copyright 2018 The KubeVirt Authors.
Copyright 2024 Flant JSC.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

Initially copied from https://github.com/kubevirt/kubevirt/blob/main/pkg/virtctl/scp/scp.go
*/

package scp

import (
	"github.com/spf13/cobra"

	"k8s.io/client-go/tools/clientcmd"

	"github.com/deckhouse/virtualization/src/pkg/cli/internal/cmd/ssh"
	"github.com/deckhouse/virtualization/src/pkg/cli/internal/templates"
)

const (
	recursiveFlag, recursiveFlagShort = "recursive", "r"
	preserveFlag                      = "preserve"
)

func NewCommand(clientConfig clientcmd.ClientConfig) *cobra.Command {
	c := &SCP{
		clientConfig: clientConfig,
		options:      ssh.DefaultSSHOptions(),
	}
	c.options.LocalClientName = "scp"

	cmd := &cobra.Command{
		Use:     "scp VirtualMachine)",
		Short:   "SCP files from/to a virtual machine.",
		Example: usage(),
		Args:    templates.ExactArgs("scp", 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return c.Run(cmd, args)
		},
	}

	ssh.AddCommandlineArgs(cmd.Flags(), &c.options)
	cmd.Flags().BoolVarP(&c.recursive, recursiveFlag, recursiveFlagShort, c.recursive,
		"Recursively copy entire directories")
	cmd.Flags().BoolVar(&c.preserve, preserveFlag, c.preserve,
		"Preserves modification times, access times, and modes from the original file.")
	cmd.SetUsageTemplate(templates.UsageTemplate())
	return cmd
}

type SCP struct {
	clientConfig clientcmd.ClientConfig
	options      ssh.SSHOptions
	recursive    bool
	preserve     bool
}

func (o *SCP) Run(cmd *cobra.Command, args []string) error {
	local, remote, toRemote, err := PrepareCommand(cmd, o.clientConfig, &o.options, args)
	if err != nil {
		return err
	}

	if o.options.WrapLocalSSH {
		clientArgs := o.buildSCPTarget(local, remote, toRemote)
		return ssh.RunLocalClient(cmd, remote.Namespace, remote.Name, &o.options, clientArgs)
	}

	return o.nativeSCP(local, remote, toRemote)
}

func PrepareCommand(cmd *cobra.Command, clientConfig clientcmd.ClientConfig, opts *ssh.SSHOptions, args []string) (local templates.LocalSCPArgument, remote templates.RemoteSCPArgument, toRemote bool, err error) {
	opts.IdentityFilePathProvided = cmd.Flags().Changed(ssh.IdentityFilePathFlag)
	local, remote, toRemote, err = templates.ParseSCPArguments(args[0], args[1])
	if err != nil {
		return
	}

	if len(remote.Namespace) < 1 {
		remote.Namespace, _, err = clientConfig.Namespace()
		if err != nil {
			return
		}
	}

	if len(remote.Username) > 0 {
		opts.SSHUsername = remote.Username
	}
	return
}

func usage() string {
	return `  # Copy a file to the remote home folder of user "user"
  {{ProgramName}} scp myfile.bin user@myvm:myfile.bin

  # Copy a directory to the remote home folder of user "user"
  {{ProgramName}} scp --recursive ~/mydir/ user@myvm:./mydir

  # Copy a file to the remote home folder of user "user" without specifying a file name on the target
  {{ProgramName}} scp myfile.bin user@myvm:.

  # Copy a file to 'myvm' in 'mynamespace' namespace
  {{ProgramName}} scp myfile.bin user@myvm.mynamespace:myfile.bin

  # Copy a file from the remote location to a local folder
  {{ProgramName}} scp user@myvm:myfile.bin ~/myfile.bin`
}
