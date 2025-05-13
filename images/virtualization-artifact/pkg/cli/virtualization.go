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

Initially copied from https://github.com/kubevirt/kubevirt/blob/main/pkg/virtctl/root.go
*/

package virtualization

import (
	"os"
	"strings"

	"github.com/spf13/cobra"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/component-base/logs"

	"github.com/deckhouse/deckhouse-cli/internal/virtualization/cmd/console"
	"github.com/deckhouse/deckhouse-cli/internal/virtualization/cmd/lifecycle"
	"github.com/deckhouse/deckhouse-cli/internal/virtualization/cmd/portforward"
	"github.com/deckhouse/deckhouse-cli/internal/virtualization/cmd/scp"
	"github.com/deckhouse/deckhouse-cli/internal/virtualization/cmd/ssh"
	"github.com/deckhouse/deckhouse-cli/internal/virtualization/cmd/vnc"

	"github.com/deckhouse/virtualization/api/client/kubeclient"

	"github.com/deckhouse/deckhouse-cli/internal/virtualization/templates"
)

func NewCommand(programName string) (*cobra.Command, clientcmd.ClientConfig) {
	// programName used in cobra templates to display either `d8 virtualization` or `d8vctl`
	cobra.AddTemplateFunc(
		"ProgramName", func() string {
			return programName
		},
	)

	// used to enable replacement of `ProgramName` placeholder for cobra.Example, which has no template support
	cobra.AddTemplateFunc(
		"prepare", func(s string) string {
			result := strings.Replace(s, "{{ProgramName}}", programName, -1)
			return result
		},
	)

	virtCmd := &cobra.Command{
		Use:           programName,
		Short:         programName + " controls virtual machine related operations on your kubernetes cluster.",
		SilenceUsage:  true,
		SilenceErrors: true,
		Run: func(cmd *cobra.Command, args []string) {
			cmd.Help()
		},
	}

	logs.AddFlags(virtCmd.PersistentFlags())

	virtCmd.SetUsageTemplate(templates.MainUsageTemplate())
	virtCmd.SetOut(os.Stdout)

	optionsCmd := &cobra.Command{
		Use:    "options",
		Hidden: true,
		Run: func(cmd *cobra.Command, args []string) {
			cmd.Printf(cmd.UsageString())
		},
	}

	optionsCmd.SetUsageTemplate(templates.OptionsUsageTemplate())

	clientConfig := kubeclient.DefaultClientConfig(virtCmd.PersistentFlags())
	virtCmd.AddCommand(
		console.NewCommand(clientConfig),
		vnc.NewCommand(clientConfig),
		portforward.NewCommand(clientConfig),
		ssh.NewCommand(clientConfig),
		scp.NewCommand(clientConfig),
		lifecycle.NewStartCommand(clientConfig),
		lifecycle.NewStopCommand(clientConfig),
		lifecycle.NewRestartCommand(clientConfig),
		lifecycle.NewEvictCommand(clientConfig),
		optionsCmd,
	)
	return virtCmd, clientConfig
}
