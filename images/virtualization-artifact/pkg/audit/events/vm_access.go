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

package events

import (
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apiserver/pkg/apis/audit"
	"k8s.io/client-go/tools/cache"

	"github.com/deckhouse/deckhouse/pkg/log"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type NewVMConnectOptions struct {
	VMInformer   cache.Indexer
	VDInformer   cache.Indexer
	NodeInformer cache.Indexer
}

func NewVMConnect(options NewVMConnectOptions) *VMAccess {
	return &VMAccess{
		vmInformer:   options.VMInformer,
		nodeInformer: options.NodeInformer,
		vdInformer:   options.VDInformer,
	}
}

type VMAccess struct {
	vmInformer   cache.Indexer
	vdInformer   cache.Indexer
	nodeInformer cache.Indexer
}

func (m *VMAccess) IsMatched(event *audit.Event) bool {
	if event.Stage != audit.StageResponseComplete || event.ObjectRef == nil {
		return false
	}

	if event.ObjectRef.Resource != "virtualmachines" || event.ObjectRef.APIGroup != "subresources.virtualization.deckhouse.io" {
		return false
	}

	if event.ObjectRef.Subresource == "console" || event.ObjectRef.Subresource == "vnc" || event.ObjectRef.Subresource == "portforward" {
		return true
	}

	return false
}

func (m *VMAccess) Log(event *audit.Event) error {
	response := map[string]string{
		"type":           "Access to VM",
		"level":          "info",
		"name":           "unknown",
		"datetime":       event.RequestReceivedTimestamp.Format(time.RFC3339),
		"uid":            string(event.AuditID),
		"requestSubject": event.User.Username,

		"action-type":          event.Verb,
		"node-network-address": "unknown",
		"virtualmachine-uid":   "unknown",
		"virtualmachine-os":    "unknown",
		"storageclasses":       "unknown",
		"qemu-version":         "unknown",
		"libvirt-version":      "unknown",

		"operation-result": event.Annotations["authorization.k8s.io/decision"],
	}

	switch event.ObjectRef.Subresource {
	case "console":
		response["name"] = "Access to VM via serial console"
	case "vnc":
		response["name"] = "Access to VM via VNC"
	case "portforward":
		response["name"] = "Access to VM via portforward"
	}

	vm, err := m.getVMFromInformer(event)
	if err != nil {
		return fmt.Errorf("fail to get vm from informer: %w", err)
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

	response["virtualmachine-uid"] = string(vm.UID)
	response["virtualmachine-os"] = vm.Status.GuestOSInfo.Name

	logSlice := []any{}
	for k, v := range response {
		logSlice = append(logSlice, slog.String(k, v))
	}
	log.Info("VMConnect", logSlice...)

	return nil
}

func (m *VMAccess) getVMFromInformer(event *audit.Event) (*v1alpha2.VirtualMachine, error) {
	vmObj, exist, err := m.vmInformer.GetByKey(event.ObjectRef.Namespace + "/" + event.ObjectRef.Name)
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

func (m *VMAccess) fillVDInfo(response map[string]string, vm *v1alpha2.VirtualMachine) error {
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
		response["storageclasses"] = strings.Join(slices.Compact(storageClasses), ",")
	}

	return nil
}

func (m *VMAccess) fillNodeInfo(response map[string]string, vm *v1alpha2.VirtualMachine) error {
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
	for _, addr := range node.Status.Addresses {
		if addr.Type != corev1.NodeHostName && addr.Address != "" {
			addresses = append(addresses, addr.Address)
		}
	}

	if len(addresses) != 0 {
		response["node-network-address"] = strings.Join(slices.Compact(addresses), ",")
	}

	return nil
}
