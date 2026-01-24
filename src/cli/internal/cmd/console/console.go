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

	// Set terminal to raw mode once for all connections
	if term.IsTerminal(int(os.Stdin.Fd())) {
		state, err := term.MakeRaw(int(os.Stdin.Fd()))
		if err != nil {
			return fmt.Errorf("make raw terminal failed: %w", err)
		}
		defer func() {
			_ = term.Restore(int(os.Stdin.Fd()), state)
		}()
	}

	// Channel for stdin data - created once for all connections
	// to avoid losing characters during reconnection
	stdinCh := make(chan []byte, 100)
	doneChan := make(chan struct{}, 1)

	go func() {
		defer close(stdinCh)
		in := os.Stdin
		buf := make([]byte, 1024)
		for {
			n, err := in.Read(buf)
			if err != nil {
				if err != io.EOF || n == 0 {
					return
				}
			}

			if n > 0 {
				// Escape sequence Ctrl+] (^]) - code 29 - exit console
				if buf[0] == 29 {
					doneChan <- struct{}{}
					return
				}

				// Copy data to avoid losing it on the next read
				data := make([]byte, n)
				copy(data, buf[0:n])
				stdinCh <- data
			}
		}
	}()

	firstConnection := true
	showedReconnectMessage := false

	for {
		select {
		case <-cmd.Context().Done():
			return nil
		case <-doneChan:
			return nil
		default:
			if firstConnection {
				fmt.Fprintf(os.Stderr, "Connecting to %s console...\r\n", name)
			}

			err := connect(cmd.Context(), name, namespace, client, c.timeout, stdinCh, doneChan)
			if err != nil {
				if strings.Contains(err.Error(), "not found") {
					return err
				}

				// If VM is not in Running state, show reconnection message
				if strings.Contains(err.Error(), "not Running") || strings.Contains(err.Error(), "not Running or Migrating") {
					if !firstConnection {
						if !showedReconnectMessage {
							fmt.Fprintf(os.Stderr, "\r\nConnection lost. Reconnecting")
							showedReconnectMessage = true
						}
						// Show progress dots
						fmt.Fprintf(os.Stderr, ".")
					}
					time.Sleep(time.Second)
					firstConnection = false
					continue
				}

				var e *websocket.CloseError
				if errors.As(err, &e) {
					switch e.Code {
					case websocket.CloseGoingAway:
						if showedReconnectMessage {
							fmt.Fprintf(os.Stderr, "\r\n")
						}
						fmt.Fprintf(os.Stderr, util.CloseGoingAwayMessage)
						return nil
					case websocket.CloseAbnormalClosure:
						if !firstConnection {
							if !showedReconnectMessage {
								fmt.Fprintf(os.Stderr, "\r\nConnection lost. Reconnecting")
								showedReconnectMessage = true
							}
							fmt.Fprintf(os.Stderr, ".")
						}
					}
				} else {
					if showedReconnectMessage {
						fmt.Fprintf(os.Stderr, "\r\n")
					}
					fmt.Fprintf(os.Stderr, "\r\n%s\r\n", err)
					showedReconnectMessage = false
				}

				time.Sleep(time.Second)
				firstConnection = false
			} else {
				// connect returned nil - normal exit (escape sequence)
				return nil
			}
		}
	}
}

func connect(ctx context.Context, name, namespace string, virtCli kubeclient.Client, timeout int, stdinCh <-chan []byte, doneChan <-chan struct{}) error {
	// in -> stdinWriter | stdinReader -> console
	// out <- stdoutReader | stdoutWriter <- console
	stdinReader, stdinWriter := io.Pipe()
	stdoutReader, stdoutWriter := io.Pipe()

	// Unbuffered channels - as in original
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

	fmt.Fprintf(os.Stderr, "\r\nSuccessfully connected to %s console. The escape sequence is ^]\r\n", name)

	out := os.Stdout
	go func() {
		_, err := io.Copy(out, stdoutReader)
		if err != nil {
			readStopErr <- err
		}
	}()

	// Channel to signal write goroutine termination
	writeCtx, writeCancel := context.WithCancel(ctx)
	writeDone := make(chan struct{})

	go func() {
		defer close(writeDone)
		_, err := stdinWriter.Write([]byte("\r"))
		if err != nil {
			if errors.Is(err, io.EOF) {
				return
			}
			writeStopErr <- err
			return
		}

		for {
			select {
			case <-writeCtx.Done():
				return
			case <-doneChan:
				return
			case b, ok := <-stdinCh:
				if !ok {
					return
				}
				_, err = stdinWriter.Write(b)
				if err != nil {
					if errors.Is(err, io.EOF) {
						return
					}
					writeStopErr <- err
					return
				}
			}
		}
	}()

	var result error
	select {
	case <-ctx.Done():
		result = ctx.Err()
	case <-doneChan:
		result = nil
	case err = <-k8sResErr:
		result = err
	case err = <-writeStopErr:
		result = err
	case err = <-readStopErr:
		result = err
	}

	// Terminate write goroutine and wait for completion
	writeCancel()
	// Close pipes to unblock goroutines
	_ = stdinWriter.Close()
	_ = stdoutWriter.Close()
	_ = stdinReader.Close()
	_ = stdoutReader.Close()
	<-writeDone

	return result
}
