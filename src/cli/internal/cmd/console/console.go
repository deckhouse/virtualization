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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

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

	cmd.Flags().IntVar(&console.timeout, "timeout", 300, "The number of seconds to wait for the virtual machine to be ready.")
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
  # Configure 60 seconds timeout (default 300 seconds)
  {{ProgramName}} console --timeout=60 myvm`

	return usage
}

var spinnerChars = []rune{'⠋', '⠙', '⠹', '⠸', '⠼', '⠴', '⠦', '⠧', '⠇', '⠏'}

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

	showedWaitMessage := false
	spinnerIdx := 0
	startTime := time.Now()
	timeout := time.Duration(c.timeout) * time.Second

	for {
		select {
		case <-cmd.Context().Done():
			return nil
		case <-doneChan:
			return nil
		default:
			err := connect(cmd.Context(), name, namespace, client, c.timeout, stdinCh, doneChan, &startTime)
			if err == nil {
				return nil // Normal exit (escape sequence)
			}

			if strings.Contains(err.Error(), "not found") {
				return err
			}

			// Check if we should show waiting message and retry
			errMsg := err.Error()
			shouldWait := strings.Contains(errMsg, "not Running") ||
				strings.Contains(errMsg, "bad handshake") ||
				strings.Contains(errMsg, "Internal error")
			var wsErr *websocket.CloseError
			if errors.As(err, &wsErr) {
				if wsErr.Code == websocket.CloseGoingAway {
					if showedWaitMessage {
						fmt.Fprintf(os.Stderr, "\r\x1b[K")
					}
					fmt.Fprintf(os.Stderr, util.CloseGoingAwayMessage)
					return nil
				}
				shouldWait = shouldWait || wsErr.Code == websocket.CloseAbnormalClosure
			}

			if shouldWait {
				// Check total timeout
				if time.Since(startTime) > timeout {
					if showedWaitMessage {
						fmt.Fprintf(os.Stderr, "\r\x1b[K")
					}
					return fmt.Errorf("timeout after %d second(s) waiting for VirtualMachine %q serial console", c.timeout, name)
				}

				// Get VM phase and show waiting spinner
				phase := "Unknown"
				if vm, vmErr := client.VirtualMachines(namespace).Get(cmd.Context(), name, metav1.GetOptions{}); vmErr == nil {
					phase = string(vm.Status.Phase)
				}
				fmt.Fprintf(os.Stderr, "\r\x1b[K%c Waiting for VirtualMachine %q serial console. Current VM phase: %s. Press Ctrl+] to exit.",
					spinnerChars[spinnerIdx], name, phase)
				spinnerIdx = (spinnerIdx + 1) % len(spinnerChars)
				showedWaitMessage = true
				time.Sleep(200 * time.Millisecond)
				continue
			}

			// Unknown error - print and continue
			if showedWaitMessage {
				fmt.Fprintf(os.Stderr, "\r\x1b[K")
			}
			fmt.Fprintf(os.Stderr, "\r\n%s\r\n", err)
			showedWaitMessage = false
			time.Sleep(time.Second)
		}
	}
}

func connect(ctx context.Context, name, namespace string, virtCli kubeclient.Client, timeout int, stdinCh <-chan []byte, doneChan <-chan struct{}, startTime *time.Time) error {
	// in -> stdinWriter | stdinReader -> console
	// out <- stdoutReader | stdoutWriter <- console
	stdinReader, stdinWriter := io.Pipe()
	stdoutReader, stdoutWriter := io.Pipe()

	// Unbuffered channels - as in original
	k8sResErr := make(chan error)
	writeStopErr := make(chan error)
	readStopErr := make(chan error)

	console, err := virtCli.VirtualMachines(namespace).SerialConsole(name, &virtualizationv1alpha2.SerialConsoleOptions{ConnectionTimeout: time.Duration(timeout) * time.Second})
	if err != nil {
		return fmt.Errorf("can't access VM %s: %w", name, err)
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

	// Clear spinner line and show success message
	fmt.Fprintf(os.Stderr, "\r\x1b[K\r\nSuccessfully connected to %s serial console. Press Ctrl+] to exit.\r\n", name)
	// Reset timeout after successful connection
	if startTime != nil {
		*startTime = time.Now()
	}

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
