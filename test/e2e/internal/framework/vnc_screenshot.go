/*
Copyright 2025 Flant JSC

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
	"context"
	"encoding/binary"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"net"
	"os"
	"path"
	"slices"
	"time"

	. "github.com/onsi/ginkgo/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

const (
	vncScreenshotTimeout    = 30 * time.Second
	vncScreenshotAttempts   = 3
	vncScreenshotRetryDelay = 2 * time.Second
)

// vmHasLiveDomain reports whether a VirtualMachine in this phase is expected to
// have a running libvirt domain, i.e. whether VNC / serial console capture is
// meaningful. This is the phase filter shared by every guest-console artifact.
func vmHasLiveDomain(phase v1alpha2.MachinePhase) bool {
	switch phase {
	case v1alpha2.MachineRunning,
		v1alpha2.MachineDegraded,
		v1alpha2.MachineStarting,
		v1alpha2.MachineStopping,
		v1alpha2.MachineMigrating,
		v1alpha2.MachinePause:
		return true
	default:
		return false
	}
}

// saveVMScreenshots captures a VNC screenshot of every VM with a live domain in
// the test namespace. A screenshot is the primary way to see where a guest is
// stuck when it boots but never brings up SSH and the guest agent, and the
// virt-launcher pod dies with the namespace, so it must be captured here.
//
// Capture is retried, and on persistent failure an explicit *_screen_error.log
// breadcrumb is written into the bundle: a silently missing screenshot (as
// happened for a wedged guest whose agent never came up) leaves the next
// investigation blind, so the reason for a missing frame must be recorded.
func (f *Framework) saveVMScreenshots(ctx context.Context, dumpDir string) {
	vms, err := f.Clients.VirtClient().VirtualMachines(f.Namespace().Name).List(ctx, metav1.ListOptions{})
	if err != nil {
		GinkgoWriter.Printf("Failed to list VirtualMachines for screenshots:\nError: %v\n", err)
		return
	}

	for _, vm := range vms.Items {
		if !vmHasLiveDomain(vm.Status.Phase) {
			continue
		}

		fileName := path.Join(dumpDir, fmt.Sprintf("vm_%s_screen.png", vm.Name))
		var lastErr error
		for attempt := 1; attempt <= vncScreenshotAttempts; attempt++ {
			lastErr = f.captureVNCScreenshot(vm.Name, fileName)
			if lastErr == nil {
				break
			}
			GinkgoWriter.Printf("VNC screenshot attempt %d/%d failed:\nVirtualMachine: %s\nError: %v\n", attempt, vncScreenshotAttempts, vm.Name, lastErr)
			if attempt < vncScreenshotAttempts {
				time.Sleep(vncScreenshotRetryDelay)
			}
		}
		if lastErr != nil {
			errFile := path.Join(dumpDir, fmt.Sprintf("vm_%s_screen_error.log", vm.Name))
			msg := fmt.Sprintf("failed to capture VNC screenshot for VirtualMachine %q (phase %s) after %d attempts: %v\n", vm.Name, vm.Status.Phase, vncScreenshotAttempts, lastErr)
			if werr := os.WriteFile(errFile, []byte(msg), 0o644); werr != nil {
				GinkgoWriter.Printf("Failed to write VNC screenshot error breadcrumb:\nFile: %s\nError: %v\n", errFile, werr)
			}
		}
	}
}

func (f *Framework) captureVNCScreenshot(vmName, fileName string) error {
	stream, err := f.Clients.VirtClient().VirtualMachines(f.Namespace().Name).VNC(vmName)
	if err != nil {
		return fmt.Errorf("open VNC stream: %w", err)
	}

	conn := stream.AsConn()
	defer conn.Close()

	type result struct {
		img *image.RGBA
		err error
	}
	resultChan := make(chan result, 1)
	go func() {
		img, err := grabRFBFramebuffer(conn)
		resultChan <- result{img: img, err: err}
	}()

	select {
	case res := <-resultChan:
		if res.err != nil {
			return res.err
		}
		file, err := os.Create(fileName)
		if err != nil {
			return fmt.Errorf("create screenshot file: %w", err)
		}
		defer file.Close()
		return png.Encode(file, res.img)
	case <-time.After(vncScreenshotTimeout):
		// Unblock the reader goroutine.
		conn.Close()
		return fmt.Errorf("timed out after %s", vncScreenshotTimeout)
	}
}

// grabRFBFramebuffer speaks just enough RFB 3.8 (VNC) to fetch a single full
// framebuffer in raw encoding: handshake with no authentication, request one
// non-incremental update, and assemble the rectangles into an image.
func grabRFBFramebuffer(conn net.Conn) (*image.RGBA, error) {
	const (
		rfbVersion         = "RFB 003.008\n"
		securityTypeNone   = 1
		encodingRaw        = 0
		msgFramebufferUpd  = 0
		msgSetColourMap    = 1
		msgBell            = 2
		msgServerCutText   = 3
		bytesPerPixel      = 4
		maxHandshakeErrLen = 1024
	)

	buf := make([]byte, 12)
	if _, err := io.ReadFull(conn, buf); err != nil {
		return nil, fmt.Errorf("read server version: %w", err)
	}
	if _, err := conn.Write([]byte(rfbVersion)); err != nil {
		return nil, fmt.Errorf("send client version: %w", err)
	}

	if _, err := io.ReadFull(conn, buf[:1]); err != nil {
		return nil, fmt.Errorf("read security types count: %w", err)
	}
	nsec := int(buf[0])
	if nsec == 0 {
		var reasonLen uint32
		if err := binary.Read(conn, binary.BigEndian, &reasonLen); err != nil {
			return nil, fmt.Errorf("read handshake error length: %w", err)
		}
		if reasonLen > maxHandshakeErrLen {
			reasonLen = maxHandshakeErrLen
		}
		reason := make([]byte, reasonLen)
		_, _ = io.ReadFull(conn, reason)
		return nil, fmt.Errorf("server refused handshake: %s", reason)
	}
	secTypes := make([]byte, nsec)
	if _, err := io.ReadFull(conn, secTypes); err != nil {
		return nil, fmt.Errorf("read security types: %w", err)
	}
	if !slices.Contains(secTypes, byte(securityTypeNone)) {
		return nil, fmt.Errorf("server offers no None security type: %v", secTypes)
	}
	if _, err := conn.Write([]byte{securityTypeNone}); err != nil {
		return nil, fmt.Errorf("send security type: %w", err)
	}
	var secResult uint32
	if err := binary.Read(conn, binary.BigEndian, &secResult); err != nil {
		return nil, fmt.Errorf("read security result: %w", err)
	}
	if secResult != 0 {
		return nil, fmt.Errorf("security handshake failed: %d", secResult)
	}

	// ClientInit: shared.
	if _, err := conn.Write([]byte{1}); err != nil {
		return nil, fmt.Errorf("send ClientInit: %w", err)
	}
	var width, height uint16
	if err := binary.Read(conn, binary.BigEndian, &width); err != nil {
		return nil, fmt.Errorf("read framebuffer width: %w", err)
	}
	if err := binary.Read(conn, binary.BigEndian, &height); err != nil {
		return nil, fmt.Errorf("read framebuffer height: %w", err)
	}
	// Server pixel format (16) + name length (4) + name.
	if _, err := io.ReadFull(conn, make([]byte, 16)); err != nil {
		return nil, fmt.Errorf("read server pixel format: %w", err)
	}
	var nameLen uint32
	if err := binary.Read(conn, binary.BigEndian, &nameLen); err != nil {
		return nil, fmt.Errorf("read desktop name length: %w", err)
	}
	if _, err := io.CopyN(io.Discard, conn, int64(nameLen)); err != nil {
		return nil, fmt.Errorf("read desktop name: %w", err)
	}

	// SetPixelFormat: 32bpp, depth 24, little-endian, true color, RGB shifts 16/8/0.
	setPixelFormat := []byte{
		0, 0, 0, 0, // message type + padding
		32, 24, 0, 1, // bpp, depth, big-endian, true-color
		0, 255, 0, 255, 0, 255, // max R, G, B (uint16 each)
		16, 8, 0, // shift R, G, B
		0, 0, 0, // padding
	}
	if _, err := conn.Write(setPixelFormat); err != nil {
		return nil, fmt.Errorf("send SetPixelFormat: %w", err)
	}
	// SetEncodings: raw only.
	if _, err := conn.Write([]byte{2, 0, 0, 1, 0, 0, 0, byte(encodingRaw)}); err != nil {
		return nil, fmt.Errorf("send SetEncodings: %w", err)
	}
	// FramebufferUpdateRequest: full screen, non-incremental.
	updReq := make([]byte, 10)
	updReq[0] = 3
	binary.BigEndian.PutUint16(updReq[6:], width)
	binary.BigEndian.PutUint16(updReq[8:], height)
	if _, err := conn.Write(updReq); err != nil {
		return nil, fmt.Errorf("send FramebufferUpdateRequest: %w", err)
	}

	img := image.NewRGBA(image.Rect(0, 0, int(width), int(height)))
	remaining := int(width) * int(height)
	for remaining > 0 {
		if _, err := io.ReadFull(conn, buf[:1]); err != nil {
			return nil, fmt.Errorf("read server message type: %w", err)
		}
		switch buf[0] {
		case msgFramebufferUpd:
			var pad [1]byte
			if _, err := io.ReadFull(conn, pad[:]); err != nil {
				return nil, err
			}
			var nrects uint16
			if err := binary.Read(conn, binary.BigEndian, &nrects); err != nil {
				return nil, err
			}
			for range nrects {
				rectHeader := make([]byte, 12)
				if _, err := io.ReadFull(conn, rectHeader); err != nil {
					return nil, err
				}
				rx := int(binary.BigEndian.Uint16(rectHeader[0:]))
				ry := int(binary.BigEndian.Uint16(rectHeader[2:]))
				rw := int(binary.BigEndian.Uint16(rectHeader[4:]))
				rh := int(binary.BigEndian.Uint16(rectHeader[6:]))
				encoding := int32(binary.BigEndian.Uint32(rectHeader[8:]))
				if encoding != encodingRaw {
					return nil, fmt.Errorf("unexpected encoding %d", encoding)
				}
				rectData := make([]byte, rw*rh*bytesPerPixel)
				if _, err := io.ReadFull(conn, rectData); err != nil {
					return nil, err
				}
				for row := range rh {
					for col := range rw {
						offset := (row*rw + col) * bytesPerPixel
						// Little-endian BGRX with shifts 16/8/0.
						img.SetRGBA(rx+col, ry+row, color.RGBA{R: rectData[offset+2], G: rectData[offset+1], B: rectData[offset], A: 255})
					}
				}
				remaining -= rw * rh
			}
		case msgSetColourMap:
			header := make([]byte, 5)
			if _, err := io.ReadFull(conn, header); err != nil {
				return nil, err
			}
			ncolours := int(binary.BigEndian.Uint16(header[3:]))
			if _, err := io.CopyN(io.Discard, conn, int64(ncolours*6)); err != nil {
				return nil, err
			}
		case msgBell:
		case msgServerCutText:
			header := make([]byte, 7)
			if _, err := io.ReadFull(conn, header); err != nil {
				return nil, err
			}
			textLen := binary.BigEndian.Uint32(header[3:])
			if _, err := io.CopyN(io.Discard, conn, int64(textLen)); err != nil {
				return nil, err
			}
		default:
			return nil, fmt.Errorf("unexpected server message type %d", buf[0])
		}
	}

	return img, nil
}
