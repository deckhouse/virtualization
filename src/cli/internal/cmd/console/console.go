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
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/spf13/cobra"
	"golang.org/x/term"

	virtualizationv1alpha2 "github.com/deckhouse/virtualization/api/client/generated/clientset/versioned/typed/core/v1alpha2"
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

	for {
		select {
		case <-cmd.Context().Done():
			return nil
		default:
			cmd.Printf("Connecting to %s console...\n", name)

			err := connect(cmd.Context(), name, namespace, client, c.timeout)
			if err != nil {
				if strings.Contains(err.Error(), "not found") {
					return err
				}

				var e *websocket.CloseError
				if errors.As(err, &e) {
					switch e.Code {
					case websocket.CloseGoingAway:
						cmd.Printf(util.CloseGoingAwayMessage)
						return nil
					case websocket.CloseAbnormalClosure:
						cmd.Printf(util.CloseAbnormalClosureMessage)
					}
				} else {
					cmd.Printf("%s\n", err)
				}

				time.Sleep(time.Second)
			} else {
				return nil
			}
		}
	}
}

func connect(ctx context.Context, name string, namespace string, virtCli kubeclient.Client, timeout int) error {
	// in -> stdinWriter | stdinReader -> console
	// out <- stdoutReader | stdoutWriter <- console
	stdinReader, stdinWriter := io.Pipe()
	stdoutReader, stdoutWriter := io.Pipe()

	doneChan := make(chan struct{}, 1)

	k8sResErr := make(chan error)
	writeStopErr := make(chan error)
	readStopErr := make(chan error)

	console, err := virtCli.VirtualMachines(namespace).SerialConsole(name, &virtualizationv1alpha2.SerialConsoleOptions{ConnectionTimeout: time.Duration(timeout) * time.Minute})
	if err != nil {
		return fmt.Errorf("can't access VM %s: %s", name, err.Error())
	}

	go func() {
		err := console.Stream(virtualizationv1alpha2.StreamOptions{
			In:  stdinReader,
			Out: stdoutWriter,
		})
		if err != nil {
			k8sResErr <- err
		}
	}()

	if term.IsTerminal(int(os.Stdin.Fd())) {
		state, err := term.MakeRaw(int(os.Stdin.Fd()))
		if err != nil {
			return fmt.Errorf("make raw terminal failed: %w", err)
		}
		defer term.Restore(int(os.Stdin.Fd()), state)
	}

	fmt.Fprintf(os.Stderr, "Successfully connected to %s console. The escape sequence is ^]\n", name)

	out := os.Stdout
	go func() {
		_, err := io.Copy(out, stdoutReader)
		if err != nil {
			readStopErr <- err
		}
	}()

	stdinCh := make(chan []byte)
	go func() {
		in := os.Stdin
		buf := make([]byte, 1024)
		for {
			// reading from stdin
			n, err := in.Read(buf)
			if err != nil {
				if err != io.EOF || n == 0 {
					return
				}

				readStopErr <- err
			}

			// the escape sequence
			if buf[0] == 29 {
				doneChan <- struct{}{}
				return
			}

			stdinCh <- buf[0:n]
		}
	}()

	go func() {
		_, err := stdinWriter.Write([]byte("\r"))
		if err != nil {
			if err == io.EOF {
				return
			}

			writeStopErr <- err
		}

		for b := range stdinCh {
			_, err = stdinWriter.Write(b)
			if err != nil {
				if err == io.EOF {
					return
				}

				writeStopErr <- err
			}
		}
	}()

	select {
	case <-ctx.Done():
	case <-doneChan:
	case err = <-k8sResErr:
	case err = <-writeStopErr:
	case err = <-readStopErr:
	}

	return err
}
