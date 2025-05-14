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
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/spf13/cobra"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/deckhouse/virtualization/api/client/kubeclient"
	"github.com/deckhouse/virtualization/src/pkg/cli/internal/templates"
	"github.com/deckhouse/virtualization/src/pkg/cli/internal/util"
)

var timeout int

func NewCommand(clientConfig clientcmd.ClientConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "console (VirtualMachine)",
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
  {{ProgramName}} console myvm.mynamespace
  {{ProgramName}} console myvm -n mynamespace
  # Configure one minute timeout (default 5 minutes)
  {{ProgramName}} console --timeout=1 myvm`

	return usage
}

func (c *Console) Run(args []string) error {
	namespace, name, err := templates.ParseTarget(args[0])
	if err != nil {
		return err
	}
	if namespace == "" {
		namespace, _, err = c.clientConfig.Namespace()
		if err != nil {
			return err
		}
	}

	virtCli, err := kubeclient.GetClientFromClientConfig(c.clientConfig)
	if err != nil {
		return err
	}

	for {
		err := connect(name, namespace, virtCli)
		if err != nil {
			if errors.Is(err, util.ErrorInterrupt) || strings.Contains(err.Error(), "not found") {
				return err
			}

			var e *websocket.CloseError
			if errors.As(err, &e) {
				switch e.Code {
				case websocket.CloseGoingAway:
					fmt.Fprint(os.Stderr, "\nYou were disconnected from the console. This has one of the following reasons:"+
						"\n - another user connected to the console of the target vm\n")
					return nil
				case websocket.CloseAbnormalClosure:
					fmt.Fprint(os.Stderr, "\nYou were disconnected from the console. This has one of the following reasons:"+
						"\n - network issues"+
						"\n - machine restart\n")
				}
			} else {
				fmt.Fprintf(os.Stderr, "%s\n", err)
			}

			time.Sleep(time.Second)
		}
	}
}

func connect(name string, namespace string, virtCli kubeclient.Client) error {
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
