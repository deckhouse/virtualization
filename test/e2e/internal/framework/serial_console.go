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

package framework

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"time"

	. "github.com/onsi/ginkgo/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	genv1alpha2 "github.com/deckhouse/virtualization/api/client/generated/clientset/versioned/typed/core/v1alpha2"
)

const (
	serialConsoleConnectTimeout = 15 * time.Second
	serialConsoleReadTimeout    = 10 * time.Second
	// serialConsoleMaxBytes caps a single capture so a chatty console cannot
	// grow the bundle without bound.
	serialConsoleMaxBytes = 256 * 1024
)

// saveVMSerialConsoles captures the serial console output of every VM with a
// live domain in the test namespace. When a guest boots but never brings up the
// guest agent / SSH (agent stays not-ready for minutes on a VM that is Running),
// the VNC screenshot only shows the final frame — the serial console shows the
// kernel and init output that pins WHERE boot wedged. The e2e-br image runs a
// getty on ttyS0, so a nudge (newline) yields either the last messages or the
// login prompt. The virt-launcher pod dies with the namespace, so this must be
// captured here, before cleanup.
func (f *Framework) saveVMSerialConsoles(ctx context.Context, dumpDir string) {
	vms, err := f.Clients.VirtClient().VirtualMachines(f.Namespace().Name).List(ctx, metav1.ListOptions{})
	if err != nil {
		GinkgoWriter.Printf("Failed to list VirtualMachines for serial console:\nError: %v\n", err)
		return
	}

	for _, vm := range vms.Items {
		if !vmHasLiveDomain(vm.Status.Phase) {
			continue
		}

		fileName := path.Join(dumpDir, fmt.Sprintf("vm_%s_serial.log", vm.Name))
		if err := f.captureSerialConsole(vm.Name, fileName); err != nil {
			GinkgoWriter.Printf("Failed to capture serial console:\nVirtualMachine: %s\nError: %v\n", vm.Name, err)
			// Leave an explicit breadcrumb so a missing serial log is explained.
			msg := fmt.Sprintf("failed to capture serial console for VirtualMachine %q (phase %s): %v\n", vm.Name, vm.Status.Phase, err)
			if werr := os.WriteFile(fileName, []byte(msg), 0o644); werr != nil {
				GinkgoWriter.Printf("Failed to write serial console error breadcrumb:\nFile: %s\nError: %v\n", fileName, werr)
			}
		}
	}
}

// captureSerialConsole opens the serial console, nudges it with a newline and
// records whatever the guest streams within serialConsoleReadTimeout. A live
// console never reaches EOF, so a read timeout is the normal path — whatever was
// buffered is still written.
func (f *Framework) captureSerialConsole(vmName, fileName string) error {
	stream, err := f.Clients.VirtClient().VirtualMachines(f.Namespace().Name).SerialConsole(
		vmName,
		&genv1alpha2.SerialConsoleOptions{ConnectionTimeout: serialConsoleConnectTimeout},
	)
	if err != nil {
		return fmt.Errorf("open serial console stream: %w", err)
	}

	conn := stream.AsConn()
	defer conn.Close()

	// Nudge the console so a wedged-but-alive getty/kernel prints a fresh line
	// (login prompt or the last message) instead of leaving an idle, empty stream.
	_, _ = conn.Write([]byte("\n"))

	type result struct {
		data []byte
		err  error
	}
	resultChan := make(chan result, 1)
	go func() {
		var buf bytes.Buffer
		_, copyErr := io.CopyN(&buf, conn, serialConsoleMaxBytes)
		if errors.Is(copyErr, io.EOF) || errors.Is(copyErr, io.ErrUnexpectedEOF) {
			copyErr = nil
		}
		resultChan <- result{data: buf.Bytes(), err: copyErr}
	}()

	writeCaptured := func(data []byte) error {
		if werr := os.WriteFile(fileName, data, 0o644); werr != nil {
			return fmt.Errorf("write serial console file: %w", werr)
		}
		return nil
	}

	select {
	case res := <-resultChan:
		if werr := writeCaptured(res.data); werr != nil {
			return werr
		}
		return res.err
	case <-time.After(serialConsoleReadTimeout):
		// Expected path for a live console: close to unblock the reader, then
		// persist whatever streamed within the window.
		conn.Close()
		res := <-resultChan
		return writeCaptured(res.data)
	}
}
