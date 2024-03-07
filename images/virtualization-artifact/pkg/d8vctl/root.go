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

package d8vctl

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/deckhouse/virtualization-controller/api/client/kubecli"
	"github.com/deckhouse/virtualization-controller/pkg/d8vctl/console"
	"github.com/deckhouse/virtualization-controller/pkg/d8vctl/portforward"
	"github.com/deckhouse/virtualization-controller/pkg/d8vctl/templates"
	"github.com/deckhouse/virtualization-controller/pkg/d8vctl/vnc"
)

func NewD8vctlCommand() (*cobra.Command, clientcmd.ClientConfig) {
	programName := GetProgramName(filepath.Base(os.Args[0]))

	// used in cobra templates to display either `kubectl d8virt` or `d8vctl`
	cobra.AddTemplateFunc(
		"ProgramName", func() string {
			return programName
		},
	)

	// used to enable replacement of `ProgramName` placeholder for cobra.Example, which has no template support
	cobra.AddTemplateFunc(
		"prepare", func(s string) string {
			// order matters!
			result := strings.Replace(s, "kubectl", "kubectl d8virt", -1)
			result = strings.Replace(result, "{{ProgramName}}", programName, -1)
			return result
		},
	)

	rootCmd := &cobra.Command{
		Use:           programName,
		Short:         programName + " controls virtual machine related operations on your kubernetes cluster.",
		SilenceUsage:  true,
		SilenceErrors: true,
		Run: func(cmd *cobra.Command, args []string) {
			cmd.Printf(cmd.UsageString())
		},
	}

	optionsCmd := &cobra.Command{
		Use:    "options",
		Hidden: true,
		Run: func(cmd *cobra.Command, args []string) {
			cmd.Printf(cmd.UsageString())
		},
	}
	optionsCmd.SetUsageTemplate(templates.OptionsUsageTemplate())

	clientConfig := kubecli.DefaultClientConfig(rootCmd.PersistentFlags())
	AddGlogFlags(rootCmd.PersistentFlags())
	rootCmd.SetUsageTemplate(templates.MainUsageTemplate())
	rootCmd.SetOut(os.Stdout)
	rootCmd.AddCommand(
		console.NewCommand(clientConfig),
		vnc.NewCommand(clientConfig),
		portforward.NewCommand(clientConfig),
		optionsCmd,
	)
	return rootCmd, clientConfig
}

func GetProgramName(binary string) string {
	if strings.HasSuffix(binary, "-d8virt") {
		return fmt.Sprintf("%s d8virt", strings.TrimSuffix(binary, "-d8virt"))
	}
	return binary
}

func Execute() {
	cmd, _ := NewD8vctlCommand()
	if err := cmd.Execute(); err != nil {
		fmt.Fprintln(cmd.Root().ErrOrStderr(), strings.TrimSpace(err.Error()))
		os.Exit(1)
	}
}
