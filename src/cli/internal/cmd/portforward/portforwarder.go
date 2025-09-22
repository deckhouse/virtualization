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

Initially copied from https://github.com/kubevirt/kubevirt/blob/main/pkg/virtctl/portforward/portforwarder.go
*/

package portforward

import (
	"errors"
	"net"
	"strings"

	"k8s.io/klog/v2"

	virtualizationv1alpha2 "github.com/deckhouse/virtualization/api/client/generated/clientset/versioned/typed/core/v1alpha2"
	subv1alpha3 "github.com/deckhouse/virtualization/api/subresources/v1alpha3"
)

type portForwarder struct {
	namespace string
	name      string
	resource  portforwardableResource
}

type portforwardableResource interface {
	PortForward(name string, options subv1alpha3.VirtualMachinePortForward) (virtualizationv1alpha2.StreamInterface, error)
}

func (p *portForwarder) startForwarding(address *net.IPAddr, port forwardedPort) error {
	klog.Infof("forwarding %s %s:%d to %d", port.protocol, address, port.local, port.remote)
	if port.protocol == protocolUDP {
		return p.startForwardingUDP(address, port)
	}

	if port.protocol == protocolTCP {
		return p.startForwardingTCP(address, port)
	}

	return errors.New("unknown protocol: " + port.protocol)
}

func handleConnectionError(err error, port forwardedPort) {
	if err != nil && !strings.Contains(err.Error(), "use of closed network connection") {
		klog.Errorf("error handling connection for %d: %v", port.local, err)
	}
}
