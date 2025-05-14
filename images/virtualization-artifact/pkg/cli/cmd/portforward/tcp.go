/*
Copyright 2018 The Kubernetes Authors.
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

Initially copied from https://github.com/kubevirt/kubevirt/blob/main/pkg/virtctl/portforward/tcp.go
*/

package portforward

import (
	"io"
	"net"

	"k8s.io/klog/v2"

	"github.com/deckhouse/virtualization/api/subresources/v1alpha2"
)

func (p *portForwarder) startForwardingTCP(address *net.IPAddr, port forwardedPort) error {
	listener, err := net.ListenTCP(
		port.protocol,
		&net.TCPAddr{
			IP:   address.IP,
			Zone: address.Zone,
			Port: port.local,
		})
	if err != nil {
		return err
	}

	go p.waitForConnection(listener, port)

	return nil
}

func (p *portForwarder) waitForConnection(listener net.Listener, port forwardedPort) {
	for {
		conn, err := listener.Accept()
		if err != nil {
			klog.Errorln("error accepting connection:", err)
			return
		}
		klog.Infof("opening new tcp tunnel to %d", port.remote)
		stream, err := p.resource.PortForward(p.name, v1alpha2.VirtualMachinePortForward{Port: port.remote, Protocol: port.protocol})
		if err != nil {
			klog.Errorf("can't access vm/%s.%s: %v", p.name, p.namespace, err)
			return
		}
		go p.handleConnection(conn, stream.AsConn(), port)
	}
}

// handleConnection copies data between the local connection and the stream to
// the remote server.
func (p *portForwarder) handleConnection(local, remote net.Conn, port forwardedPort) {
	klog.Infof("handling tcp connection for %d", port.local)
	errs := make(chan error)
	go func() {
		_, err := io.Copy(remote, local)
		errs <- err
	}()
	go func() {
		_, err := io.Copy(local, remote)
		errs <- err
	}()

	handleConnectionError(<-errs, port)
	local.Close()
	remote.Close()
	handleConnectionError(<-errs, port)
}
