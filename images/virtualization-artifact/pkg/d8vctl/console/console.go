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

Initially copied from https://github.com/kubevirt/kubevirt/blob/main/pkg/virtctl/console/console.go
*/

package console

import (
	"fmt"
	"io"
	"os"
	"os/signal"
	"time"

	"github.com/gorilla/websocket"
	"github.com/spf13/cobra"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/deckhouse/virtualization-controller/api/client/kubecli"
	"github.com/deckhouse/virtualization-controller/pkg/d8vctl/templates"
	"github.com/deckhouse/virtualization-controller/pkg/d8vctl/util"
)

var timeout int

func NewCommand(clientConfig clientcmd.ClientConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "console (VM)",
		Short:   "Connect to a console of a virtual machine.",
		Example: usage(),
		Args:    templates.ExactArgs("console", 1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c := Console{clientConfig: clientConfig}
			return c.Run(args)
		},
	}

	cmd.Flags().IntVar(&timeout, "timeout", 5, "The number of minutes to wait for the virtual machine to be ready.")
	cmd.SetUsageTemplate(templates.UsageTemplate())
	return cmd
}

type Console struct {
	clientConfig clientcmd.ClientConfig
}

func usage() string {
	usage := `  # Connect to the console on VirtualMachine 'myvm':
  {{ProgramName}} console myvm
  # Configure one minute timeout (default 5 minutes)
  {{ProgramName}} console --timeout=1 myvm`

	return usage
}

func (c *Console) Run(args []string) error {
	namespace, _, err := c.clientConfig.Namespace()
	if err != nil {
		return err
	}

	vm := args[0]

	virtCli, err := kubecli.GetClientFromClientConfig(c.clientConfig)
	if err != nil {
		return err
	}

	stdinReader, stdinWriter := io.Pipe()
	stdoutReader, stdoutWriter := io.Pipe()

	// in -> stdinWriter | stdinReader -> console
	// out <- stdoutReader | stdoutWriter <- console
	// Wait until the virtual machine is in running phase, user interrupt or timeout
	resChan := make(chan error)
	runningChan := make(chan error)
	waitInterrupt := make(chan os.Signal, 1)
	signal.Notify(waitInterrupt, os.Interrupt)

	go func() {
		con, err := virtCli.VirtualMachines(namespace).SerialConsole(vm, &kubecli.SerialConsoleOptions{ConnectionTimeout: time.Duration(timeout) * time.Minute})
		runningChan <- err

		if err != nil {
			return
		}

		resChan <- con.Stream(kubecli.StreamOptions{
			In:  stdinReader,
			Out: stdoutWriter,
		})
	}()

	select {
	case <-waitInterrupt:
		// Make a new line in the terminal
		fmt.Println()
		return nil
	case err = <-runningChan:
		if err != nil {
			return err
		}
	}
	err = util.AttachConsole(stdinReader, stdoutReader, stdinWriter, stdoutWriter,
		fmt.Sprint("Successfully connected to ", vm, " console. The escape sequence is ^]\n"),
		resChan)

	if err != nil {
		if e, ok := err.(*websocket.CloseError); ok && e.Code == websocket.CloseAbnormalClosure {
			fmt.Fprint(os.Stderr, "\nYou were disconnected from the console. This has one of the following reasons:"+
				"\n - another user connected to the console of the target vm"+
				"\n - network issues\n")
		}
		return err
	}
	return nil
}
