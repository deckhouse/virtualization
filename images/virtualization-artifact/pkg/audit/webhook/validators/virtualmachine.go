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
	"strings"

	admissionv1 "k8s.io/api/admission/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/tools/cache"

	"github.com/deckhouse/deckhouse/pkg/log"
	"github.com/deckhouse/virtualization-controller/pkg/audit/webhook"
	"github.com/deckhouse/virtualization-controller/pkg/audit/webhook/util"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type NewVirtualMachineWebhookOptions struct {
	VMInformer   cache.Indexer
	VDInformer   cache.Indexer
	NodeInformer cache.Indexer
}

func NewVirtualMachineWebhook(options NewVirtualMachineWebhookOptions) *VirtualMachineWebhook {
	return &VirtualMachineWebhook{
		vmInformer:   options.VMInformer,
		nodeInformer: options.NodeInformer,
		vdInformer:   options.VDInformer,
		response: &admissionv1.AdmissionResponse{
			AuditAnnotations: map[string]string{},
		},
	}
}

type VirtualMachineWebhook struct {
	vmInformer   cache.Indexer
	vdInformer   cache.Indexer
	nodeInformer cache.Indexer
	response     *admissionv1.AdmissionResponse
}

func (m *VirtualMachineWebhook) Path() string {
	return "/virtualmachine"
}

func (m *VirtualMachineWebhook) Handler() http.Handler {
	return webhook.NewAuditWebhookHandler(m.Validate)
}

func (m *VirtualMachineWebhook) Validate(ar *admissionv1.AdmissionReview) *admissionv1.AdmissionResponse {
	vm, err := m.getVM(ar)
	if err != nil {
		log.Error("fail to get vm from admission review", log.Err(err))
		return m.response
	}

	if len(vm.Status.BlockDeviceRefs) > 0 {
		if err := m.fillVDInfo(vm); err != nil {
			log.Error("fail to fill vd info", log.Err(err))
		}
	}

	if vm.Status.Node != "" {
		if err := m.fillNodeInfo(vm); err != nil {
			log.Error("fail to fill node info", log.Err(err))
		}
	}

	m.response.AuditAnnotations["virtualmachine-uid"] = string(vm.UID)
	m.response.AuditAnnotations["virtualmachine-os"] = vm.Status.GuestOSInfo.Name

	log.Info(
		"VirtualMachine",
		slog.Any("AuditAnnotations", m.response.AuditAnnotations),
	)

	return m.response
}

func (m *VirtualMachineWebhook) getVM(ar *admissionv1.AdmissionReview) (*v1alpha2.VirtualMachine, error) {
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

	return vm, err
}
func (m *VirtualMachineWebhook) getVMFromInformer(ar *admissionv1.AdmissionReview) (*v1alpha2.VirtualMachine, error) {
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

func (m *VirtualMachineWebhook) fillVDInfo(vm *v1alpha2.VirtualMachine) error {
	storageClasses := make([]string, len(vm.Spec.BlockDeviceRefs))

	for _, bd := range vm.Spec.BlockDeviceRefs {
		vdObj, exist, err := m.vdInformer.GetByKey(vm.Namespace + "/" + bd.Name)
		if err != nil {
			return fmt.Errorf("fail to get node from informer: %w", err)
		}
		if !exist {
			return errors.New("vmObj not exist")
		}

		vd, ok := vdObj.(*v1alpha2.VirtualDisk)
		if !ok {
			return errors.New("fail to convert vmObj to vm")
		}

		storageClasses = append(storageClasses, vd.Status.StorageClassName)
	}

	m.response.AuditAnnotations["storageclasses"] = strings.Join(storageClasses, ",")

	return nil
}

func (m *VirtualMachineWebhook) fillNodeInfo(vm *v1alpha2.VirtualMachine) error {
	nodeObj, exist, err := m.nodeInformer.GetByKey(vm.Status.Node)
	if err != nil {
		return fmt.Errorf("fail to get node from informer: %w", err)
	}
	if !exist {
		return errors.New("nodeObj not exist")
	}
	node, ok := nodeObj.(*corev1.Node)
	if !ok {
		return errors.New("fail to convert nodeObj to node")
	}

	addresses := ""
	for i, r := range node.Status.Addresses {
		addresses += r.Address
		if i != len(node.Status.Addresses)-1 {
			addresses += ","
		}
	}

	m.response.AuditAnnotations["node-network-address"] = addresses

	return nil
}
