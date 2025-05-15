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

Initially copied from https://github.com/kubevirt/kubevirt/blob/main/pkg/virtctl/templates/templates.go
*/

package templates

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/deckhouse/virtualization/api/client/kubeclient"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

// UsageTemplate returns the usage template for all subcommands
func UsageTemplate() string {
	return `Usage:{{if .Runnable}}
  {{prepare .UseLine}}{{end}}{{if .HasAvailableSubCommands}}
  {{prepare .CommandPath}} [command]{{end}}{{if gt (len .Aliases) 0}}

Aliases:
  {{.NameAndAliases}}{{end}}{{if .HasExample}}

Examples:
{{prepare .Example}}{{end}}{{if .HasAvailableSubCommands}}

Available Commands:{{range .Commands}}{{if (or .IsAvailableCommand (eq .Name "help"))}}
  {{rpad .Name .NamePadding }} {{prepare .Short}}{{end}}{{end}}{{end}}{{if .HasAvailableLocalFlags}}

Flags:
{{.LocalFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}{{if .HasAvailableInheritedFlags}}

Use "{{ProgramName}} options" for a list of global command-line options (applies to all commands).{{end}}
`
}

// MainUsageTemplate returns the usage template for the root command
func MainUsageTemplate() string {
	return `Available Commands:{{range .Commands}}{{if (or .IsAvailableCommand (eq .Name "help"))}}
  {{rpad .Name .NamePadding }} {{prepare .Short}}{{end}}{{end}}

Use "{{ProgramName}} <command> --help" for more information about a given command.
Use "{{ProgramName}} options" for a list of global command-line options (applies to all commands).
`
}

// OptionsUsageTemplate returns a template which prints all global available commands
func OptionsUsageTemplate() string {
	return `The following options can be passed to any command:{{if .HasAvailableInheritedFlags}}

{{.InheritedFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}
`
}

// ExactArgs validate the number of input parameters
func ExactArgs(nameOfCommand string, n int) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if len(args) == n {
			return nil
		}
		err := errors.New("argument validation failed")
		_, e := fmt.Fprintf(os.Stderr, "fatal: Number of input parameters is incorrect, %s accepts %d arg(s), received %d\n\n", nameOfCommand, n, len(args))
		if e != nil {
			err = errors.Join(err, e)
		}
		e = cmd.Help()
		if e != nil {
			err = errors.Join(err, e)
		}
		return err
	}
}

// PrintWarningForPausedVM prints warning message if VM is paused
func PrintWarningForPausedVM(virtCli kubeclient.Client, vmName, namespace string) {
	vm, err := virtCli.VirtualMachines(namespace).Get(context.Background(), vmName, metav1.GetOptions{})
	if err != nil {
		return
	}
	if vm.Status.Phase == virtv2.MachinePause {
		fmt.Fprintf(os.Stderr, "\rWarning: %s is paused. Console will be active after unpause.\n", vmName)
	}
}
