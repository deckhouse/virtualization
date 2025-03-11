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

package validators

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"slices"
	"strings"

	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/cache"

	"github.com/deckhouse/deckhouse/pkg/log"
	"github.com/deckhouse/virtualization-controller/pkg/audit/webhook"
	"github.com/deckhouse/virtualization-controller/pkg/audit/webhook/util"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type NewPortForwardWebhookOptions struct {
	VMInformer   cache.Indexer
	VDInformer   cache.Indexer
	NodeInformer cache.Indexer
}

func NewPortForwardWebhook(options NewPortForwardWebhookOptions) *PortForwardWebhook {
	return &PortForwardWebhook{
		vmInformer:   options.VMInformer,
		nodeInformer: options.NodeInformer,
		vdInformer:   options.VDInformer,
	}
}

type PortForwardWebhook struct {
	vmInformer   cache.Indexer
	vdInformer   cache.Indexer
	nodeInformer cache.Indexer
}

func (m *PortForwardWebhook) Path() string {
	return "/portforward"
}

func (m *PortForwardWebhook) Handler() http.Handler {
	return webhook.NewAuditWebhookHandler(m.Validate)
}

func (m *PortForwardWebhook) Validate(ar *admissionv1.AdmissionReview) (*admissionv1.AdmissionResponse, error) {
	response := &admissionv1.AdmissionResponse{
		AuditAnnotations: map[string]string{
			"node-network-address": "unknown",
			"virtualmachine-uid":   "unknown",
			"virtualmachine-os":    "unknown",
			"storageclasses":       "unknown",
			"qemu-version":         "unknown",
			"libvirt-version":      "unknown",
		},
	}

	vm, err := m.getVM(ar)
	if err != nil {
		return response, fmt.Errorf("fail to get vm from admission review: %w", err)
	}

	if len(vm.Spec.BlockDeviceRefs) > 0 {
		if err := m.fillVDInfo(response, vm); err != nil {
			log.Error("fail to fill vd info", log.Err(err))
		}
	}

	if vm.Status.Node != "" {
		if err := m.fillNodeInfo(response, vm); err != nil {
			log.Error("fail to fill node info", log.Err(err))
		}
	}

	response.AuditAnnotations["virtualmachine-uid"] = string(vm.UID)
	response.AuditAnnotations["virtualmachine-os"] = vm.Status.GuestOSInfo.Name
	// TODO: Populate these fields from the node
	// m.response.AuditAnnotations["qemu-version"] = ""
	// m.response.AuditAnnotations["libvirt-version"] = ""

	log.Info(
		"VirtualMachine",
		slog.Any("AuditAnnotations", response.AuditAnnotations),
	)

	return response, nil
}

func (m *PortForwardWebhook) getVM(ar *admissionv1.AdmissionReview) (*v1alpha2.VirtualMachine, error) {
	if len(ar.Request.Object.Raw) > 0 {
		vm, _, err := util.GetVMFromAdmissionReview(ar)
		if err != nil {
			return nil, fmt.Errorf("fail to get vm from admission review: %w", err)
		}

		return vm, err
	}

	vm, err := m.getVMFromInformer(ar)
	if err != nil {
		return nil, fmt.Errorf("fail to get vm from informer: %w", err)
	}

	return vm, nil
}

func (m *PortForwardWebhook) getVMFromInformer(ar *admissionv1.AdmissionReview) (*v1alpha2.VirtualMachine, error) {
	vmObj, exist, err := m.vmInformer.GetByKey(ar.Request.Namespace + "/" + ar.Request.Name)
	if err != nil {
		return nil, fmt.Errorf("fail to get node from informer: %w", err)
	}
	if !exist {
		return nil, errors.New("vmObj not exist")
	}

	vm, ok := vmObj.(*v1alpha2.VirtualMachine)
	if !ok {
		return nil, errors.New("fail to convert vmObj to vm")
	}

	return vm, nil
}

func (m *PortForwardWebhook) fillVDInfo(response *admissionv1.AdmissionResponse, vm *v1alpha2.VirtualMachine) error {
	storageClasses := []string{}

	for _, bd := range vm.Spec.BlockDeviceRefs {
		if bd.Kind != v1alpha2.VirtualDiskKind {
			continue
		}

		vdObj, exist, err := m.vdInformer.GetByKey(vm.Namespace + "/" + bd.Name)
		if err != nil {
			return fmt.Errorf("fail to get virtual disk from informer: %w", err)
		}
		if !exist {
			continue
		}

		vd, ok := vdObj.(*v1alpha2.VirtualDisk)
		if !ok {
			return errors.New("fail to convert vdObj to vd")
		}

		storageClasses = append(storageClasses, vd.Status.StorageClassName)
	}

	if len(storageClasses) != 0 {
		response.AuditAnnotations["storageclasses"] = strings.Join(slices.Compact(storageClasses), ",")
	}

	return nil
}

func (m *PortForwardWebhook) fillNodeInfo(response *admissionv1.AdmissionResponse, vm *v1alpha2.VirtualMachine) error {
	nodeObj, exist, err := m.nodeInformer.GetByKey(vm.Status.Node)
	if err != nil {
		return fmt.Errorf("fail to get node from informer: %w", err)
	}
	if !exist {
		return nil
	}
	node, ok := nodeObj.(*corev1.Node)
	if !ok {
		return errors.New("fail to convert nodeObj to node")
	}

	addresses := []string{}
	for _, r := range node.Status.Addresses {
		if r.Type == corev1.NodeHostName {
			continue
		}

		addresses = append(addresses, r.Address)
	}

	if len(addresses) != 0 {
		response.AuditAnnotations["node-network-address"] = strings.Join(slices.Compact(addresses), ",")
	}

	return nil
}
