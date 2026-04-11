/*
Copyright 2018 The KubeVirt Authors
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

Initially copied from https://github.com/kubevirt/kubevirt/blob/main/pkg/virtctl/ssh/ssh.go
*/

package ssh

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/deckhouse/virtualization/src/cli/internal/clientconfig"
	"github.com/deckhouse/virtualization/src/cli/internal/templates"
)

const (
	KnownHostsFileName                              = "d8virtualization_known_hosts"
	portFlag, portFlagShort                         = "port", "p"
	usernameFlag, usernameFlagShort                 = "username", "l"
	IdentityFilePathFlag, identityFilePathFlagShort = "identity-file", "i"
	knownHostsFilePathFlag                          = "known-hosts"
	commandToExecute, commandToExecuteShort         = "command", "c"
	additionalOpts, additionalOptsShort             = "local-ssh-opts", "t"
	wrapLocalSSHFlag                                = "local-ssh"
	wrapLocalSSHDefault                             = false
)

type SSH struct {
	options SSHOptions
	command string
}

type SSHOptions struct {
	SSHPort                   int
	SSHUsername               string
	IdentityFilePath          string
	IdentityFilePathProvided  bool
	KnownHostsFilePath        string
	KnownHostsFilePathDefault string
	AdditionalSSHLocalOptions []string
	WrapLocalSSH              bool
	LocalClientName           string
}

func DefaultSSHOptions() SSHOptions {

	options := SSHOptions{
		SSHPort:                   22,
		SSHUsername:               defaultUsername(),
		IdentityFilePath:          filepath.Join("~", ".ssh", "id_rsa"),
		IdentityFilePathProvided:  false,
		KnownHostsFilePath:        "",
		KnownHostsFilePathDefault: filepath.Join("~", ".ssh", KnownHostsFileName),
		AdditionalSSHLocalOptions: []string{},
		WrapLocalSSH:              wrapLocalSSHDefault,
		LocalClientName:           "ssh",
	}

	return options
}

func (s *SSHOptions) ResolvePaths() error {
	if s.IdentityFilePath != "" {
		resolvedPath, err := resolveHomeDir(s.IdentityFilePath)
		if err != nil {
			return fmt.Errorf("resolve identity file path '%s': %w", s.IdentityFilePath, err)
		}
		s.IdentityFilePath = resolvedPath
	}
	if s.KnownHostsFilePath != "" {
		resolvedPath, err := resolveHomeDir(s.KnownHostsFilePath)
		if err != nil {
			return fmt.Errorf("resolve known hosts file path '%s': %w", s.KnownHostsFilePath, err)
		}
		s.KnownHostsFilePath = resolvedPath
	}
	return nil
}

// resolveHomeDir substitutes beginning '~' with home dir path.
func resolveHomeDir(path string) (string, error) {
	if !strings.HasPrefix(path, "~") {
		return path, nil
	}
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("get user home directory: %w", err)
	}
	return filepath.Join(homeDir, strings.TrimPrefix(path, "~")), nil
}

func defaultUsername() string {
	vars := []string{
		"USER",     // linux
		"USERNAME", // linux, windows
		"LOGNAME",  // linux
	}
	for _, env := range vars {
		if v := os.Getenv(env); v != "" {
			return v
		}
	}
	return ""
}

func NewCommand() *cobra.Command {
	c := &SSH{
		options: DefaultSSHOptions(),
	}

	cmd := &cobra.Command{
		Use:     "ssh [-n|--namespace NAMESPACE] VIRTUAL-MACHINE-NAME",
		Short:   "Open a SSH connection to a virtual machine.",
		Example: usage(),
		Args:    templates.ExactArgs("ssh", 1),
		RunE:    c.Run,
	}

	AddCommonSSHFlags(cmd.Flags(), &c.options)

	cmd.Flags().StringVarP(&c.options.SSHUsername, usernameFlag, usernameFlagShort, c.options.SSHUsername,
		"Specify user to log into virtual machine; If unassigned, this will be empty and the SSH default will apply")
	cmd.Flags().StringVarP(&c.command, commandToExecute, commandToExecuteShort, c.command,
		"Specify a command to execute in the VM.")
	cmd.SetUsageTemplate(templates.UsageTemplate())
	return cmd
}

func AddCommonSSHFlags(flagset *pflag.FlagSet, opts *SSHOptions) {
	flagset.StringVarP(&opts.IdentityFilePath, IdentityFilePathFlag, identityFilePathFlagShort, opts.IdentityFilePath,
		"Specify a path to a private key used for authenticating to the server; If not provided, the client will try to use the local ssh-agent at $SSH_AUTH_SOCK")
	flagset.StringVar(&opts.KnownHostsFilePath, knownHostsFilePathFlag, opts.KnownHostsFilePathDefault,
		"Set a path to the known_hosts file.")
	flagset.IntVarP(&opts.SSHPort, portFlag, portFlagShort, opts.SSHPort,
		`Specify a port to connect to`)

	addLocalSSHClientFlags(flagset, opts)
}

func (o *SSH) Run(cmd *cobra.Command, args []string) error {
	err := o.options.ResolvePaths()
	if err != nil {
		return err
	}

	client, defaultNamespace, _, err := clientconfig.ClientAndNamespaceFromContext(cmd.Context())
	if err != nil {
		return err
	}
	namespace, name, err := PrepareCommand(cmd, defaultNamespace, &o.options, args)
	if err != nil {
		return err
	}

	if o.options.WrapLocalSSH {
		clientArgs := o.buildSSHTarget(namespace, name)
		return RunLocalClient(cmd, namespace, name, &o.options, clientArgs)
	}

	return o.nativeSSH(namespace, name, client)
}

func PrepareCommand(cmd *cobra.Command, defaultNamespace string, opts *SSHOptions, args []string) (namespace, name string, err error) {
	opts.IdentityFilePathProvided = cmd.Flags().Changed(IdentityFilePathFlag)
	var targetUsername string
	namespace, name, targetUsername, err = templates.ParseSSHTarget(args[0])
	if err != nil {
		return namespace, name, err
	}

	if len(namespace) < 1 {
		namespace = defaultNamespace
	}

	if len(targetUsername) > 0 {
		opts.SSHUsername = targetUsername
	}
	return namespace, name, err
}

func usage() string {
	return fmt.Sprintf(`  # Connect to virtualMachine 'myvm' in 'vms' namespace:
  {{ProgramName}} ssh user@myvm.vms

  # Specify namespace and user with flags:
  {{ProgramName}} ssh --namespace=vms --%s=user myvm

  # Specify identity file:
  {{ProgramName}} ssh -n vms user@myvm -%s /some/path/id_rsa_for_myvm

  # Run command instead of opening shell:
  {{ProgramName}} ssh -n vms user@myvm -%s 'ls -la /'

  # Connect using the local ssh binary found in $PATH:
  {{ProgramName}} ssh --%s=true user@myvm

  # Specify additional options for local ssh:
  {{ProgramName}} ssh user@myvm --%s=true --%s='-o StrictHostKeyChecking=no' --%s='-o UserKnownHostsFile=/dev/null'
`,
		usernameFlag,
		identityFilePathFlagShort,
		commandToExecuteShort,
		wrapLocalSSHFlag,
		wrapLocalSSHFlag,
		additionalOpts,
		additionalOpts,
	)
}
