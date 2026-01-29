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

Initially copied from https://github.com/kubevirt/kubevirt/blob/main/pkg/virtctl/root.go
*/

package command

import (
	"context"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/spf13/cobra"
	"k8s.io/component-base/logs"

	"github.com/deckhouse/virtualization/api/client/kubeclient"
	"github.com/deckhouse/virtualization/src/cli/internal/clientconfig"
	"github.com/deckhouse/virtualization/src/cli/internal/cmd/ansibleinventory"
	"github.com/deckhouse/virtualization/src/cli/internal/cmd/collectdebuginfo"
	"github.com/deckhouse/virtualization/src/cli/internal/cmd/console"
	"github.com/deckhouse/virtualization/src/cli/internal/cmd/lifecycle"
	"github.com/deckhouse/virtualization/src/cli/internal/cmd/portforward"
	"github.com/deckhouse/virtualization/src/cli/internal/cmd/scp"
	"github.com/deckhouse/virtualization/src/cli/internal/cmd/ssh"
	"github.com/deckhouse/virtualization/src/cli/internal/cmd/usbredir"
	"github.com/deckhouse/virtualization/src/cli/internal/cmd/vnc"
	"github.com/deckhouse/virtualization/src/cli/internal/comp"
	"github.com/deckhouse/virtualization/src/cli/internal/templates"
)

func NewCommand(programName string) *cobra.Command {
	// programName used in cobra templates to display either `d8 virtualization` or `d8vctl`
	cobra.AddTemplateFunc(
		"ProgramName", func() string {
			return programName
		},
	)

	// used to enable replacement of `ProgramName` placeholder for cobra.Example, which has no template support
	cobra.AddTemplateFunc(
		"prepare", func(s string) string {
			result := strings.ReplaceAll(s, "{{ProgramName}}", programName)
			return result
		},
	)

	virtCmd := &cobra.Command{
		Use:           programName,
		Short:         programName + " controls virtual machine related operations on your kubernetes cluster.",
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}

	logs.AddFlags(virtCmd.PersistentFlags())

	virtCmd.SetUsageTemplate(templates.MainUsageTemplate())
	virtCmd.SetOut(os.Stdout)

	optionsCmd := &cobra.Command{
		Use:    "options",
		Hidden: true,
		Run: func(cmd *cobra.Command, args []string) {
			cmd.Printf("%s", cmd.UsageString())
		},
	}

	optionsCmd.SetUsageTemplate(templates.OptionsUsageTemplate())

	virtCmd.AddCommand(
		ansibleinventory.NewCommand(),
		console.NewCommand(),
		collectdebuginfo.NewCommand(),
		vnc.NewCommand(),
		portforward.NewCommand(),
		ssh.NewCommand(),
		scp.NewCommand(),
		lifecycle.NewStartCommand(),
		lifecycle.NewStopCommand(),
		lifecycle.NewRestartCommand(),
		lifecycle.NewEvictCommand(),
		usbredir.NewCommand(),
		optionsCmd,
	)

	ctx, _ := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	ctxWithClient := clientconfig.NewContext(ctx, kubeclient.DefaultClientConfig(virtCmd.PersistentFlags()))

	virtCmd.SetContext(ctxWithClient)
	_ = virtCmd.RegisterFlagCompletionFunc("namespace", comp.NamespaceFlagCompletionFunc)

	for _, cmd := range virtCmd.Commands() {
		cmd.SetContext(ctxWithClient)
		cmd.ValidArgsFunction = comp.VirtualMachineNameCompletionFunc
	}

	return virtCmd
}
