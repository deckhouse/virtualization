/*
Copyright 2026 Flant JSC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package spice

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os/exec"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/spf13/cobra"
	"k8s.io/klog/v2"

	virtualizationv1alpha2 "github.com/deckhouse/virtualization/api/client/generated/clientset/versioned/typed/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/client/kubeclient"
	subv1alpha2 "github.com/deckhouse/virtualization/api/subresources/v1alpha2"
	"github.com/deckhouse/virtualization/src/cli/internal/clientconfig"
	"github.com/deckhouse/virtualization/src/cli/internal/session"
	"github.com/deckhouse/virtualization/src/cli/internal/templates"
	"github.com/deckhouse/virtualization/src/cli/internal/util"
)

const RemoteViewer = "remote-viewer"

var (
	listenAddress        = "127.0.0.1"
	proxyOnly            bool
	customPort           = 0
	preferredCompression string
)

// Values remote-viewer accepts for --spice-preferred-compression. Note they are
// spelled differently from the libvirt ones (auto-glz here, auto_glz in the domain).
var preferredCompressionValues = map[string]bool{
	"auto-glz": true, "auto-lz": true, "quic": true,
	"glz": true, "lz": true, "off": true,
}

func NewCommand() *cobra.Command {
	s := &SPICE{}
	cmd := &cobra.Command{
		Use:     "spice VirtualMachine",
		Short:   "Open a spice connection to a virtual machine.",
		Example: usage(),
		Args:    templates.ExactArgs("spice", 1),
		RunE:    s.Run,
	}
	cmd.Flags().StringVar(&listenAddress, "address", listenAddress,
		"--address=127.0.0.1: Setting this will change the listening address of the SPICE proxy. Example: --address=0.0.0.0 will make the proxy listen on all interfaces.")
	cmd.Flags().BoolVar(&proxyOnly, "proxy-only", proxyOnly,
		"--proxy-only=false: Setting this true will run only the spice proxy and show the port where SPICE viewers can connect")
	cmd.Flags().IntVar(&customPort, "port", customPort,
		"--port=0: Assigning a port value to this will try to run the proxy on the given port if the port is accessible; If unassigned, the proxy will run on a random port")
	cmd.Flags().StringVar(&preferredCompression, "preferred-compression", preferredCompression,
		"--preferred-compression=glz: Ask the server for this image codec: auto-glz, auto-lz, quic, glz, lz, off. Overrides what the VM is configured with, so codecs can be compared without restarting it.")
	cmd.Flags().BoolVar(&s.force, "force", false, "Connect without asking, even when somebody else is using the SPICE display.")
	cmd.SetUsageTemplate(templates.UsageTemplate())
	return cmd
}

func usage() string {
	return `  # Connect to the SPICE display of the VM called 'myvm':
  {{ProgramName}} spice myvm

  # Run only the proxy and connect with your own viewer:
  {{ProgramName}} spice myvm --proxy-only

  # Compare codecs without touching the VM:
  {{ProgramName}} spice myvm --preferred-compression=quic`
}

type SPICE struct {
	force bool
}

func (o *SPICE) Run(cmd *cobra.Command, args []string) error {
	if preferredCompression != "" && !preferredCompressionValues[preferredCompression] {
		return fmt.Errorf("unknown compression %q: expected one of auto-glz, auto-lz, quic, glz, lz, off", preferredCompression)
	}

	client, namespace, _, err := clientconfig.ClientAndNamespaceFromContext(cmd.Context())
	if err != nil {
		return err
	}
	vmName := args[0]

	// Connecting takes the display over the same way VNC does, so ask before the
	// listener is up and the viewer is started.
	if session.AskBeforeConnecting(cmd.Context(), client.VirtualMachines(namespace), vmName,
		subv1alpha2.SPICESession, o.force) == session.Abort {
		return nil
	}

	ln, err := net.Listen("tcp", fmt.Sprintf("%s:%d", listenAddress, customPort))
	if err != nil {
		return fmt.Errorf("can't listen on %s: %w", listenAddress, err)
	}
	defer ln.Close()
	port := ln.Addr().(*net.TCPAddr).Port

	ctx, cancel := context.WithCancel(cmd.Context())
	defer cancel()

	// Wait until the VM answers before handing a port to the viewer: a VM that is
	// still starting would otherwise greet the user with a failed client.
	if err := waitForVM(ctx, client, namespace, vmName, cmd); err != nil {
		return err
	}

	fatal := make(chan error, 1)
	go acceptLoop(ctx, ln, client, namespace, vmName, cmd, fatal)

	if proxyOnly {
		optionString, err := json.Marshal(struct {
			Port int `json:"port"`
		}{port})
		if err != nil {
			return err
		}
		if _, err = fmt.Fprintln(cmd.OutOrStdout(), string(optionString)); err != nil {
			return err
		}
		select {
		case <-ctx.Done():
		case err := <-fatal:
			return err
		}
		return nil
	}

	cmd.Printf("Connecting to %s SPICE...\n", vmName)

	done := make(chan error, 1)
	go func() { done <- runViewer(ctx, port) }()
	select {
	case err := <-done:
		return err
	case err := <-fatal:
		return err
	}
}

// acceptLoop is the reason this command cannot reuse the VNC one. A SPICE client
// opens a separate connection per channel — main, display, inputs, cursor, and,
// when the devices are present, sound and usbredir — so a single session shows up
// here as several connections. Each of them gets its own stream to the subresource.
//
// Reconnects also work differently from `d8 v vnc`. There a dropped connection means
// a dropped session, so the whole thing is retried in a loop. Here the listener stays
// up and the SPICE client reopens whatever channel it lost by itself, so a failure is
// only fatal when the VM is gone — that is reported through fatal and ends the command.
func acceptLoop(ctx context.Context, ln net.Listener, client kubeclient.Client, namespace, vmName string, cmd *cobra.Command, fatal chan<- error) {
	for {
		conn, err := ln.Accept()
		if err != nil {
			return // listener closed on shutdown
		}
		go func() {
			defer conn.Close()
			err := pipeChannelFunc(ctx, client, namespace, vmName, conn)
			if err == nil {
				return
			}
			// The VM disappeared — retrying channels is pointless.
			if strings.Contains(err.Error(), "not found") {
				select {
				case fatal <- err:
				case <-ctx.Done():
				}
				return
			}
			var closeErr *websocket.CloseError
			if errors.As(err, &closeErr) {
				switch closeErr.Code {
				case websocket.CloseGoingAway:
					cmd.Print(util.CloseGoingAwayMessage)
				case websocket.CloseAbnormalClosure:
					cmd.Print(util.CloseAbnormalClosureMessage)
				}
				return
			}
			klog.V(2).Infof("SPICE channel closed: %v", err)
		}()
	}
}

// waitForVM retries the subresource until the VM accepts a connection, mirroring the
// retry loop of `d8 v vnc`. A missing VM is reported at once instead of being retried.
func waitForVM(ctx context.Context, client kubeclient.Client, namespace, vmName string, cmd *cobra.Command) error {
	for {
		stream, _, err := client.VirtualMachines(namespace).SPICE(ctx, vmName, nil)
		if err == nil {
			stream.AsConn().Close()
			return nil
		}
		if strings.Contains(err.Error(), "not found") {
			return fmt.Errorf("can't access VM %s: %w", vmName, err)
		}
		cmd.Printf("%s\n", err)
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(time.Second):
		}
	}
}

var pipeChannelFunc = pipeChannel

func pipeChannel(ctx context.Context, client kubeclient.Client, namespace, vmName string, conn net.Conn) error {
	stream, _, err := client.VirtualMachines(namespace).SPICE(ctx, vmName, nil)
	if err != nil {
		return fmt.Errorf("can't access VM %s: %w", vmName, err)
	}
	return stream.Stream(virtualizationv1alpha2.StreamOptions{In: conn, Out: conn})
}

func runViewer(ctx context.Context, port int) error {
	args := make([]string, 0, 2)
	if preferredCompression != "" {
		args = append(args, "--spice-preferred-compression="+preferredCompression)
	}
	args = append(args, fmt.Sprintf("spice://%s:%d", listenAddress, port))

	viewer := exec.CommandContext(ctx, RemoteViewer, args...)
	if err := viewer.Start(); err != nil {
		return fmt.Errorf("%s not found or failed to start: %w; run with --proxy-only and connect manually", RemoteViewer, err)
	}
	return viewer.Wait()
}
