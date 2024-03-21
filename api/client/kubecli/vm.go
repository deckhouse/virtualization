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

Initially copied from https://github.com/kubevirt/kubevirt/blob/main/staging/src/kubevirt.io/client-go/kubecli/vmi.go
*/

package kubecli

import (
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"k8s.io/client-go/rest"

	virtualizationv1alpha2 "github.com/deckhouse/virtualization/api/client/generated/clientset/versioned/typed/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/subresources/v1alpha2"
)

type vm struct {
	virtualizationv1alpha2.VirtualMachineInterface
	restClient *rest.RESTClient
	config     *rest.Config
	namespace  string
	resource   string
}

type SerialConsoleOptions struct {
	ConnectionTimeout time.Duration
}

type connectionStruct struct {
	con StreamInterface
	err error
}

func (v vm) SerialConsole(name string, options *SerialConsoleOptions) (StreamInterface, error) {
	if options != nil && options.ConnectionTimeout != 0 {
		ticker := time.NewTicker(options.ConnectionTimeout)
		connectionChan := make(chan connectionStruct)

		go func() {
			for {
				select {
				case <-ticker.C:
					connectionChan <- connectionStruct{
						con: nil,
						err: fmt.Errorf("timeout trying to connect to the virtual machine instance"),
					}
					return
				default:
				}

				con, err := asyncSubresourceHelper(v.config, v.resource, v.namespace, name, "console", url.Values{})
				if err != nil {
					var asyncSubresourceError *AsyncSubresourceError
					ok := errors.As(err, &asyncSubresourceError)
					// return if response status code does not equal to 400
					if !ok || asyncSubresourceError.GetStatusCode() != http.StatusBadRequest {
						connectionChan <- connectionStruct{con: nil, err: err}
						return
					}

					time.Sleep(1 * time.Second)
					continue
				}

				connectionChan <- connectionStruct{con: con, err: nil}
				return
			}
		}()
		conStruct := <-connectionChan
		return conStruct.con, conStruct.err
	} else {
		return asyncSubresourceHelper(v.config, v.resource, v.namespace, name, "console", url.Values{})
	}
}

func (v vm) VNC(name string) (StreamInterface, error) {
	return asyncSubresourceHelper(v.config, v.resource, v.namespace, name, "vnc", url.Values{})
}

func (v vm) PortForward(name string, opts v1alpha2.VirtualMachinePortForward) (StreamInterface, error) {
	params := url.Values{}
	if opts.Port > 0 {
		params.Add("port", strconv.Itoa(opts.Port))
	}
	if opts.Protocol != "" {
		params.Add("protocol", opts.Protocol)
	}
	return asyncSubresourceHelper(v.config, v.resource, v.namespace, name, "portforward", params)
}
