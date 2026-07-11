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

package blockdevice

import (
	"context"
	"encoding/binary"
	"fmt"
	"image"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/client-go/rest"
	"k8s.io/utils/ptr"

	vdbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vd"
	vibuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vi"
	vmbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vm"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
	"github.com/deckhouse/virtualization/test/e2e/internal/object"
	vmobs "github.com/deckhouse/virtualization/test/e2e/internal/observer/vm"
	"github.com/deckhouse/virtualization/test/e2e/internal/precheck"
)

// VirtualImageFormat verifies how image formats are handled when the source is an HTTP
// data source:
//   - an ISO VirtualImage boots a VirtualMachine directly (as a CD-ROM);
//   - a qcow2 VirtualImage backs a VirtualDisk, and a VirtualMachine boots from that disk.
//
// The qcow2 spec provisions its main VirtualDisk on the WFFC StorageClass, so the precheck
// label is declared on the Describe (the spec-label validator only reads container-hierarchy
// labels, not leaf It labels).
var _ = Describe("VirtualImageFormat", Label(precheck.PrecheckDefaultStorageClass), func() {
	var (
		f   *framework.Framework
		ctx context.Context
	)

	BeforeEach(func() {
		ctx = context.Background()
		f = framework.NewFramework("")
		f.Before()
		DeferCleanup(f.After)
		setupProject(ctx, f, "vi-format")
	})

	It("boots a VirtualMachine from an iso VirtualImage and shows the installer screen", func() {
		vi := vibuilder.New(
			vibuilder.WithName("vi-iso"),
			vibuilder.WithNamespace(f.Namespace().Name),
			vibuilder.WithStorage(v1alpha2.StorageContainerRegistry),
			vibuilder.WithDataSourceHTTP(object.ImageURLUbuntuISO, nil, nil),
		)

		createVirtualImageAndWait(ctx, f, vi)

		runVirtualMachineFromImageUntilRunning(ctx, f, vi)
	})

	It("provisions a VirtualDisk from a qcow2 VirtualImage and runs a VirtualMachine with a ready agent", func() {
		vi := vibuilder.New(
			vibuilder.WithName("vi-qcow2"),
			vibuilder.WithNamespace(f.Namespace().Name),
			vibuilder.WithStorage(v1alpha2.StorageContainerRegistry),
			vibuilder.WithDataSourceHTTP(object.ImageURLCustomBIOS, nil, nil),
		)

		createVirtualImageAndWait(ctx, f, vi)

		// The disk under test is the scenario's main resource, so it lives on the WFFC
		// storage class.
		vd := object.NewVDFromVI("vd-from-vi-qcow2", f.Namespace().Name, vi,
			vdbuilder.WithStorageClass(defaultStorageClass()),
			vdbuilder.WithSize(ptr.To(resource.MustParse("400Mi"))))

		createVirtualDiskAndRunVM(ctx, f, vd)
	})
})

// runVirtualMachineFromImageUntilRunning boots a VirtualMachine from vi with a blank
// target disk and verifies the installer screen over VNC. It does not wait for the
// guest agent, which is not available when booting from CD-ROM/ISO media.
func runVirtualMachineFromImageUntilRunning(ctx context.Context, f *framework.Framework, vi *v1alpha2.VirtualImage) {
	GinkgoHelper()

	blankVD := object.NewBlankVD("vd-blank-for-iso", f.Namespace().Name, defaultStorageClass(), ptr.To(resource.MustParse("4Gi")))
	vm := object.NewMinimalVM("vm-from-vi-", f.Namespace().Name,
		vmbuilder.WithBootloader(v1alpha2.EFI),
		vmbuilder.WithCPU(2, ptr.To("100%")),
		vmbuilder.WithMemory(resource.MustParse("2Gi")),
		vmbuilder.WithProvisioning(nil),
		vmbuilder.WithRunPolicy(v1alpha2.AlwaysOnPolicy),
		vmbuilder.WithBlockDeviceRefs(v1alpha2.BlockDeviceSpecRef{
			Kind:      v1alpha2.ImageDevice,
			Name:      vi.Name,
			BootOrder: ptr.To(uint(1)),
		}, v1alpha2.BlockDeviceSpecRef{
			Kind:      v1alpha2.DiskDevice,
			Name:      blankVD.Name,
			BootOrder: ptr.To(uint(2)),
		}),
	)

	By("Creating blank VirtualDisk and VirtualMachine from the VirtualImage", func() {
		err := f.CreateWithDeferredDeletion(ctx, blankVD, vm)
		Expect(err).NotTo(HaveOccurred())
	})

	obs := vmobs.StartObserver(ctx, f, vm)
	obs.Never(vmobs.BeFailed())
	obs.Never(vmobs.HaveNoBootableDevice())

	By("Waiting for the VirtualMachine to be Running", func() {
		err := obs.WaitFor(vmobs.BeRunning(), framework.LongTimeout)
		Expect(err).NotTo(HaveOccurred())
	})

	By("Checking that the OS installer boot screen is visible over VNC", func() {
		// The live-server ISO takes a while to reach the subiquity language screen
		// (kernel, systemd, snapd mounting the installer snap) — around 45s after
		// Running even on an idle cluster, so the middle timeout is too tight when
		// specs run in parallel.
		Eventually(func(g Gomega) {
			frame, err := captureVNCFrame(ctx, vm)
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(hasUbuntuInstallerBootScreen(frame)).To(BeTrue(), "expected Ubuntu installer language selection screen")
		}).WithTimeout(framework.LongTimeout).WithPolling(5 * time.Second).Should(Succeed())
	})
}

type vncFrame struct {
	width  int
	height int
	pixels []byte
}

func captureVNCFrame(ctx context.Context, vm *v1alpha2.VirtualMachine) (*vncFrame, error) {
	GinkgoHelper()

	restConfig, err := framework.GetConfig().ClusterTransport.RestConfig()
	if err != nil {
		return nil, fmt.Errorf("get rest config: %w", err)
	}

	vncURL, err := vncWebSocketURL(restConfig, vm)
	if err != nil {
		return nil, err
	}

	tlsConfig, err := rest.TLSConfigFor(restConfig)
	if err != nil {
		return nil, fmt.Errorf("build tls config: %w", err)
	}

	dialer := websocket.Dialer{
		HandshakeTimeout: 10 * time.Second,
		Proxy:            http.ProxyFromEnvironment,
		TLSClientConfig:  tlsConfig,
	}

	dialCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	conn, resp, err := dialer.DialContext(dialCtx, vncURL.String(), vncHeaders(restConfig))
	if resp != nil && resp.Body != nil {
		defer func() {
			_ = resp.Body.Close()
		}()
	}
	if err != nil {
		return nil, fmt.Errorf("connect to VNC websocket: %w", err)
	}
	defer func() {
		_ = conn.Close()
	}()

	deadline := time.Now().Add(15 * time.Second)
	_ = conn.SetReadDeadline(deadline)
	_ = conn.SetWriteDeadline(deadline)

	rfb := &rfbStream{conn: conn}
	return rfb.captureFrame()
}

func vncWebSocketURL(restConfig *rest.Config, vm *v1alpha2.VirtualMachine) (*url.URL, error) {
	u, err := url.Parse(restConfig.Host)
	if err != nil {
		return nil, fmt.Errorf("parse API server URL: %w", err)
	}

	switch u.Scheme {
	case "https":
		u.Scheme = "wss"
	case "http":
		u.Scheme = "ws"
	default:
		return nil, fmt.Errorf("unsupported API server scheme %q", u.Scheme)
	}

	basePath := strings.TrimRight(u.Path, "/")
	u.Path = fmt.Sprintf(
		"%s/apis/subresources.virtualization.deckhouse.io/v1alpha2/namespaces/%s/virtualmachines/%s/vnc",
		basePath,
		vm.Namespace,
		vm.Name,
	)
	u.RawQuery = ""
	return u, nil
}

func vncHeaders(restConfig *rest.Config) http.Header {
	headers := http.Header{}
	if restConfig.BearerToken != "" {
		headers.Set("Authorization", "Bearer "+restConfig.BearerToken)
		return headers
	}
	if restConfig.BearerTokenFile != "" {
		token, err := os.ReadFile(restConfig.BearerTokenFile)
		if err == nil {
			headers.Set("Authorization", "Bearer "+strings.TrimSpace(string(token)))
			return headers
		}
	}
	if restConfig.Username != "" || restConfig.Password != "" {
		req := &http.Request{Header: headers}
		req.SetBasicAuth(restConfig.Username, restConfig.Password)
	}
	return headers
}

type rfbStream struct {
	conn *websocket.Conn
	buf  []byte
}

func (s *rfbStream) captureFrame() (*vncFrame, error) {
	protocol, err := s.readFull(12)
	if err != nil {
		return nil, fmt.Errorf("read RFB protocol version: %w", err)
	}
	if !strings.HasPrefix(string(protocol), "RFB ") {
		return nil, fmt.Errorf("unexpected RFB protocol version %q", string(protocol))
	}
	if err := s.write(protocol); err != nil {
		return nil, fmt.Errorf("write RFB protocol version: %w", err)
	}

	securityTypeCountRaw, err := s.readFull(1)
	if err != nil {
		return nil, fmt.Errorf("read RFB security type count: %w", err)
	}
	securityTypes, err := s.readFull(int(securityTypeCountRaw[0]))
	if err != nil {
		return nil, fmt.Errorf("read RFB security types: %w", err)
	}
	if !containsByte(securityTypes, 1) {
		return nil, fmt.Errorf("RFB server does not offer no-auth security type: %v", securityTypes)
	}
	if err := s.write([]byte{1}); err != nil {
		return nil, fmt.Errorf("select RFB no-auth security type: %w", err)
	}

	securityResult, err := s.readFull(4)
	if err != nil {
		return nil, fmt.Errorf("read RFB security result: %w", err)
	}
	if binary.BigEndian.Uint32(securityResult) != 0 {
		return nil, fmt.Errorf("RFB security handshake failed with code %d", binary.BigEndian.Uint32(securityResult))
	}

	if err := s.write([]byte{1}); err != nil {
		return nil, fmt.Errorf("write RFB shared flag: %w", err)
	}

	serverInit, err := s.readFull(24)
	if err != nil {
		return nil, fmt.Errorf("read RFB server init: %w", err)
	}
	width := int(binary.BigEndian.Uint16(serverInit[0:2]))
	height := int(binary.BigEndian.Uint16(serverInit[2:4]))
	nameLength := int(binary.BigEndian.Uint32(serverInit[20:24]))
	if _, err := s.readFull(nameLength); err != nil {
		return nil, fmt.Errorf("read RFB desktop name: %w", err)
	}

	if err := s.write(setPixelFormatMessage()); err != nil {
		return nil, fmt.Errorf("set RFB pixel format: %w", err)
	}
	if err := s.write(setRawEncodingMessage()); err != nil {
		return nil, fmt.Errorf("set RFB raw encoding: %w", err)
	}
	if err := s.write(framebufferUpdateRequest(width, height)); err != nil {
		return nil, fmt.Errorf("request RFB framebuffer update: %w", err)
	}

	return s.readFramebufferUpdate(width, height)
}

func (s *rfbStream) readFull(size int) ([]byte, error) {
	for len(s.buf) < size {
		messageType, message, err := s.conn.ReadMessage()
		if err != nil {
			return nil, err
		}
		if messageType != websocket.BinaryMessage && messageType != websocket.TextMessage {
			continue
		}
		s.buf = append(s.buf, message...)
	}

	result := s.buf[:size]
	s.buf = s.buf[size:]
	return result, nil
}

func (s *rfbStream) write(data []byte) error {
	return s.conn.WriteMessage(websocket.BinaryMessage, data)
}

func (s *rfbStream) readFramebufferUpdate(width, height int) (*vncFrame, error) {
	frame := &vncFrame{
		width:  width,
		height: height,
		pixels: make([]byte, width*height*4),
	}

	for {
		messageTypeRaw, err := s.readFull(1)
		if err != nil {
			return nil, fmt.Errorf("read RFB server message type: %w", err)
		}
		if messageTypeRaw[0] != 0 {
			continue
		}

		header, err := s.readFull(3)
		if err != nil {
			return nil, fmt.Errorf("read RFB framebuffer update header: %w", err)
		}
		rectangles := int(binary.BigEndian.Uint16(header[1:3]))
		for i := 0; i < rectangles; i++ {
			rectHeader, err := s.readFull(12)
			if err != nil {
				return nil, fmt.Errorf("read RFB rectangle header: %w", err)
			}
			x := int(binary.BigEndian.Uint16(rectHeader[0:2]))
			y := int(binary.BigEndian.Uint16(rectHeader[2:4]))
			w := int(binary.BigEndian.Uint16(rectHeader[4:6]))
			h := int(binary.BigEndian.Uint16(rectHeader[6:8]))
			encoding := int32(binary.BigEndian.Uint32(rectHeader[8:12]))
			if encoding != 0 {
				return nil, fmt.Errorf("unsupported RFB rectangle encoding %d", encoding)
			}

			raw, err := s.readFull(w * h * 4)
			if err != nil {
				return nil, fmt.Errorf("read RFB raw rectangle: %w", err)
			}
			copyRawRectangle(frame, image.Rect(x, y, x+w, y+h), raw)
		}
		return frame, nil
	}
}

func setPixelFormatMessage() []byte {
	msg := make([]byte, 20)
	msg[4] = 32
	msg[5] = 24
	msg[6] = 0
	msg[7] = 1
	binary.BigEndian.PutUint16(msg[8:10], 255)
	binary.BigEndian.PutUint16(msg[10:12], 255)
	binary.BigEndian.PutUint16(msg[12:14], 255)
	msg[14] = 16
	msg[15] = 8
	msg[16] = 0
	return msg
}

func setRawEncodingMessage() []byte {
	msg := make([]byte, 8)
	msg[0] = 2
	binary.BigEndian.PutUint16(msg[2:4], 1)
	return msg
}

func framebufferUpdateRequest(width, height int) []byte {
	msg := make([]byte, 10)
	msg[0] = 3
	binary.BigEndian.PutUint16(msg[6:8], uint16(width))
	binary.BigEndian.PutUint16(msg[8:10], uint16(height))
	return msg
}

func copyRawRectangle(frame *vncFrame, rect image.Rectangle, raw []byte) {
	for y := rect.Min.Y; y < rect.Max.Y; y++ {
		dstOffset := (y*frame.width + rect.Min.X) * 4
		srcOffset := ((y - rect.Min.Y) * rect.Dx()) * 4
		copy(frame.pixels[dstOffset:dstOffset+rect.Dx()*4], raw[srcOffset:srcOffset+rect.Dx()*4])
	}
}

func hasUbuntuInstallerBootScreen(frame *vncFrame) bool {
	if frame == nil || frame.width == 0 || frame.height == 0 {
		return false
	}

	orangePixels := 0
	for y := 0; y < min(frame.height, 40); y++ {
		for x := 0; x < frame.width; x++ {
			r, g, b := frame.rgbAt(x, y)
			if r > 180 && g > 40 && g < 140 && b < 90 {
				orangePixels++
			}
		}
	}

	greenPixels := 0
	for y := 0; y < frame.height; y++ {
		for x := 0; x < frame.width; x++ {
			r, g, b := frame.rgbAt(x, y)
			if r < 80 && g > 100 && b < 80 {
				greenPixels++
			}
		}
	}

	return orangePixels > frame.width*10 && greenPixels > frame.width*2
}

func (f *vncFrame) rgbAt(x, y int) (r, g, b byte) {
	offset := (y*f.width + x) * 4
	return f.pixels[offset+2], f.pixels[offset+1], f.pixels[offset]
}

func containsByte(values []byte, want byte) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
