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

Initially copied from https://github.com/kubevirt/kubevirt/blob/main/staging/src/kubevirt.io/client-go/kubecli/vmi.go
*/

package kubeclient

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/rest"
	virtv1 "kubevirt.io/api/core/v1"

	virtualizationv1alpha2 "github.com/deckhouse/virtualization/api/client/generated/clientset/versioned/typed/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/subresources/v1alpha3"
)

type vm struct {
	virtualizationv1alpha2.VirtualMachineInterface
	restClient *rest.RESTClient
	config     *rest.Config
	namespace  string
	resource   string
}

type connectionStruct struct {
	con virtualizationv1alpha2.StreamInterface
	err error
}

func (v vm) SerialConsole(name string, options *virtualizationv1alpha2.SerialConsoleOptions) (virtualizationv1alpha2.StreamInterface, error) {
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

func (v vm) VNC(name string) (virtualizationv1alpha2.StreamInterface, error) {
	return asyncSubresourceHelper(v.config, v.resource, v.namespace, name, "vnc", url.Values{})
}

func (v vm) PortForward(name string, opts v1alpha3.VirtualMachinePortForward) (virtualizationv1alpha2.StreamInterface, error) {
	params := url.Values{}
	if opts.Port > 0 {
		params.Add("port", strconv.Itoa(opts.Port))
	}
	if opts.Protocol != "" {
		params.Add("protocol", opts.Protocol)
	}
	return asyncSubresourceHelper(v.config, v.resource, v.namespace, name, "portforward", params)
}

func (v vm) Freeze(ctx context.Context, name string, opts v1alpha3.VirtualMachineFreeze) error {
	path := fmt.Sprintf(subresourceURLTpl, v.namespace, v.resource, name, "freeze")

	unfreezeTimeout := virtv1.FreezeUnfreezeTimeout{
		UnfreezeTimeout: &metav1.Duration{},
	}

	if opts.UnfreezeTimeout != nil {
		unfreezeTimeout.UnfreezeTimeout = opts.UnfreezeTimeout
	}

	body, err := json.Marshal(&unfreezeTimeout)
	if err != nil {
		return err
	}

	return v.restClient.Put().AbsPath(path).Body(body).Do(ctx).Error()
}

func (v vm) Unfreeze(ctx context.Context, name string) error {
	path := fmt.Sprintf(subresourceURLTpl, v.namespace, v.resource, name, "unfreeze")

	return v.restClient.Put().AbsPath(path).Do(ctx).Error()
}

func (v vm) AddVolume(ctx context.Context, name string, opts v1alpha3.VirtualMachineAddVolume) error {
	path := fmt.Sprintf(subresourceURLTpl, v.namespace, v.resource, name, "addvolume")
	return v.restClient.
		Put().
		AbsPath(path).
		Param("name", opts.Name).
		Param("volumeKind", opts.VolumeKind).
		Param("pvcName", opts.PVCName).
		Param("image", opts.Image).
		Param("serial", opts.Serial).
		Param("isCdrom", strconv.FormatBool(opts.IsCdrom)).
		Do(ctx).
		Error()
}

func (v vm) RemoveVolume(ctx context.Context, name string, opts v1alpha3.VirtualMachineRemoveVolume) error {
	path := fmt.Sprintf(subresourceURLTpl, v.namespace, v.resource, name, "removevolume")
	return v.restClient.
		Put().
		AbsPath(path).
		Param("name", opts.Name).
		Do(ctx).
		Error()
}

func (v vm) CancelEvacuation(ctx context.Context, name string, dryRun []string) error {
	path := fmt.Sprintf(subresourceURLTpl, v.namespace, v.resource, name, "cancelevacuation")
	c := v.restClient.Put().AbsPath(path)
	for _, value := range dryRun {
		c.Param("dryRun", value)
	}
	return c.Do(ctx).Error()
}
