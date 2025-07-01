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

Initially copied from https://github.com/kubevirt/kubevirt/blob/main/pkg/virtctl/console/console.go
*/

package console

import (
	"errors"
	"io"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/spf13/cobra"

	"github.com/deckhouse/virtualization/api/client/kubeclient"

	"github.com/deckhouse/virtualization/src/cli/internal/clientconfig"
	"github.com/deckhouse/virtualization/src/cli/internal/templates"
	"github.com/deckhouse/virtualization/src/cli/internal/util"
)

func NewCommand() *cobra.Command {
	console := &Console{}
	cmd := &cobra.Command{
		Use:     "console (VirtualMachine)",
		Short:   "Connect to a console of a virtual machine.",
		Example: usage(),
		Args:    templates.ExactArgs("console", 1),
		RunE:    console.Run,
	}

	cmd.Flags().IntVar(&console.timeout, "timeout", 5, "The number of minutes to wait for the virtual machine to be ready.")
	cmd.SetUsageTemplate(templates.UsageTemplate())
	return cmd
}

type Console struct {
	timeout int
}

func usage() string {
	usage := `  # Connect to the console on VirtualMachine 'myvm':
  {{ProgramName}} console myvm
  {{ProgramName}} console myvm.mynamespace
  {{ProgramName}} console myvm -n mynamespace
  # Configure one minute timeout (default 5 minutes)
  {{ProgramName}} console --timeout=1 myvm`

	return usage
}

func (c *Console) Run(cmd *cobra.Command, args []string) error {
	client, defaultNamespace, _, err := clientconfig.ClientAndNamespaceFromContext(cmd.Context())
	if err != nil {
		return err
	}
	namespace, name, err := templates.ParseTarget(args[0])
	if err != nil {
		return err
	}
	if namespace == "" {
		namespace = defaultNamespace
	}

	interrupt := make(chan os.Signal, 1)
	go func() {
		<-interrupt
		close(interrupt)
	}()
	signal.Notify(interrupt, os.Interrupt)

	for {
		err := connect(name, namespace, client, c.timeout)
		if err == nil {
			continue
		}

		if errors.Is(err, util.ErrorInterrupt) || strings.Contains(err.Error(), "not found") {
			return err
		}

		var e *websocket.CloseError
		if errors.As(err, &e) {
			switch e.Code {
			case websocket.CloseGoingAway:
				cmd.Printf("\nYou were disconnected from the console. This has one of the following reasons:" +
					"\n - another user connected to the console of the target vm\n")
				return nil
			case websocket.CloseAbnormalClosure:
				cmd.Printf("\nYou were disconnected from the console. This has one of the following reasons:" +
					"\n - network issues" +
					"\n - machine restart\n")
			}
		} else {
			cmd.Printf("%s\n", err)
		}

		select {
		case <-interrupt:
			return nil
		default:
			time.Sleep(time.Second)
		}
	}
}

func connect(name string, namespace string, virtCli kubeclient.Client, timeout int) error {
	stdinReader, stdinWriter := io.Pipe()
	stdoutReader, stdoutWriter := io.Pipe()

	// in -> stdinWriter | stdinReader -> console
	// out <- stdoutReader | stdoutWriter <- console
	// Wait until the virtual machine is in running phase, user interrupt or timeout
	resChan := make(chan error)
	runningChan := make(chan error)

	go func() {
		con, err := virtCli.VirtualMachines(namespace).SerialConsole(name, &kubeclient.SerialConsoleOptions{ConnectionTimeout: time.Duration(timeout) * time.Minute})
		runningChan <- err

		if err != nil {
			return
		}

		resChan <- con.Stream(kubeclient.StreamOptions{
			In:  stdinReader,
			Out: stdoutWriter,
		})
	}()

	err := <-runningChan
	if err != nil {
		return err
	}

	err = util.AttachConsole(stdinReader, stdoutReader, stdinWriter, stdoutWriter, name, resChan)
	return err
}
