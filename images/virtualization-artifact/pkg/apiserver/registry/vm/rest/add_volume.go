/*
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
*/

package rest

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apiserver/pkg/registry/rest"
	"k8s.io/utils/ptr"
	virtv1 "kubevirt.io/api/core/v1"

	"github.com/deckhouse/virtualization-controller/pkg/tls/certmanager"
	virtlisters "github.com/deckhouse/virtualization/api/client/generated/listers/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/subresources"
)

type AddVolumeREST struct {
	*BaseREST
}

var (
	_ rest.Storage   = &AddVolumeREST{}
	_ rest.Connecter = &AddVolumeREST{}
)

func NewAddVolumeREST(baseREST *BaseREST) *AddVolumeREST {
	return &AddVolumeREST{baseREST}
}

func (r AddVolumeREST) New() runtime.Object {
	return &subresources.VirtualMachineAddVolume{}
}

func (r AddVolumeREST) Destroy() {
}

func (r AddVolumeREST) Connect(ctx context.Context, name string, opts runtime.Object, responder rest.Responder) (http.Handler, error) {
	addVolumeOpts, ok := opts.(*subresources.VirtualMachineAddVolume)
	if !ok {
		return nil, fmt.Errorf("invalid options object: %#v", opts)
	}
	var (
		addVolumePather pather
		hooks           []mutateRequestHook
	)

	if r.requestFromKubevirt(addVolumeOpts) {
		addVolumePather = newKVVMIPather("addvolume")
	} else {
		addVolumePather = newKVVMPather("addvolume")
		h, err := r.genMutateRequestHook(addVolumeOpts)
		if err != nil {
			return nil, err
		}
		hooks = append(hooks, h)
	}
	location, transport, err := AddVolumeLocation(ctx, r.vmLister, name, r.kubevirt, r.proxyCertManager, addVolumePather)
	if err != nil {
		return nil, err
	}
	handler := newThrottledUpgradeAwareProxyHandler(location, transport, false, responder, r.kubevirt.ServiceAccount, hooks...)

	return handler, nil
}

// NewConnectOptions implements rest.Connecter interface
func (r AddVolumeREST) NewConnectOptions() (runtime.Object, bool, string) {
	return &subresources.VirtualMachineAddVolume{}, false, ""
}

// ConnectMethods implements rest.Connecter interface
func (r AddVolumeREST) ConnectMethods() []string {
	return []string{http.MethodPut}
}

func (r AddVolumeREST) requestFromKubevirt(opts *subresources.VirtualMachineAddVolume) bool {
	return opts == nil || (opts.Image == "" && opts.VolumeKind == "" && opts.PVCName == "")
}

func (r AddVolumeREST) genMutateRequestHook(opts *subresources.VirtualMachineAddVolume) (mutateRequestHook, error) {
	var dd virtv1.DiskDevice
	if opts.IsCdrom {
		dd.CDRom = &virtv1.CDRomTarget{
			Bus: virtv1.DiskBusSCSI,
		}
	} else {
		dd.Disk = &virtv1.DiskTarget{
			Bus: virtv1.DiskBusSCSI,
		}
	}
	// Skip set serial for CDROM
	serial := ""
	if !opts.IsCdrom {
		serial = opts.Serial
	}

	hotplugRequest := virtv1.AddVolumeOptions{
		Name: opts.Name,
		Disk: &virtv1.Disk{
			Name:        opts.Name,
			DiskDevice:  dd,
			Serial:      serial,
			ErrorPolicy: ptr.To(virtv1.DiskErrorPolicyReport),
		},
	}
	switch opts.VolumeKind {
	case "VirtualDisk":
		if opts.PVCName == "" {
			return nil, fmt.Errorf("must specify PVCName")
		}
		hotplugRequest.VolumeSource = &virtv1.HotplugVolumeSource{
			PersistentVolumeClaim: &virtv1.PersistentVolumeClaimVolumeSource{
				PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
					ClaimName: opts.PVCName,
				},
				Hotpluggable: true,
			},
		}
	case "VirtualImage":
		switch {
		case opts.PVCName != "" && opts.Image != "":
			return nil, fmt.Errorf("must specify only one of PersistentVolumeClaimName or Image")
		case opts.PVCName != "":
			hotplugRequest.VolumeSource = &virtv1.HotplugVolumeSource{
				PersistentVolumeClaim: &virtv1.PersistentVolumeClaimVolumeSource{
					PersistentVolumeClaimVolumeSource: corev1.PersistentVolumeClaimVolumeSource{
						ClaimName: opts.PVCName,
					},
					Hotpluggable: true,
				},
			}
		case opts.Image != "":
			hotplugRequest.VolumeSource = &virtv1.HotplugVolumeSource{
				ContainerDisk: &virtv1.ContainerDiskSource{
					Image:        opts.Image,
					Hotpluggable: true,
				},
			}
		default:
			return nil, fmt.Errorf("must specify one of PersistentVolumeClaimName or Image")
		}
	case "ClusterVirtualImage":
		if opts.Image == "" {
			return nil, fmt.Errorf("must specify Image")
		}
		hotplugRequest.VolumeSource = &virtv1.HotplugVolumeSource{
			ContainerDisk: &virtv1.ContainerDiskSource{
				Image:        opts.Image,
				Hotpluggable: true,
			},
		}
	default:
		return nil, fmt.Errorf("invalid volume kind: %s", opts.VolumeKind)
	}

	newBody, err := json.Marshal(&hotplugRequest)
	if err != nil {
		return nil, err
	}

	return func(req *http.Request) error {
		return rewriteBody(req, newBody)
	}, nil
}

func AddVolumeLocation(
	ctx context.Context,
	getter virtlisters.VirtualMachineLister,
	name string,
	kubevirt KubevirtAPIServerConfig,
	proxyCertManager certmanager.CertificateManager,
	addVolumePather pather,
) (*url.URL, *http.Transport, error) {
	return streamLocation(ctx, getter, name, addVolumePather, kubevirt, proxyCertManager)
}
