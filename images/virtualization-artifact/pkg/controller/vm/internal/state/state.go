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

package state

import (
	"context"
	"fmt"
	"sync"

	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	kvvmutil "github.com/deckhouse/virtualization-controller/pkg/common/kvvm"
	"github.com/deckhouse/virtualization-controller/pkg/common/object"
	"github.com/deckhouse/virtualization-controller/pkg/controller/powerstate"
	"github.com/deckhouse/virtualization-controller/pkg/controller/reconciler"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type VirtualMachineState interface {
	VirtualMachine() *reconciler.Resource[*virtv2.VirtualMachine, virtv2.VirtualMachineStatus]
	KVVM(ctx context.Context) (*virtv1.VirtualMachine, error)
	KVVMI(ctx context.Context) (*virtv1.VirtualMachineInstance, error)
	Pods(ctx context.Context) (*corev1.PodList, error)
	Pod(ctx context.Context) (*corev1.Pod, error)
	VirtualDisk(ctx context.Context, name string) (*virtv2.VirtualDisk, error)
	VirtualImage(ctx context.Context, name string) (*virtv2.VirtualImage, error)
	ClusterVirtualImage(ctx context.Context, name string) (*virtv2.ClusterVirtualImage, error)
	VirtualDisksByName(ctx context.Context) (map[string]*virtv2.VirtualDisk, error)
	VirtualImagesByName(ctx context.Context) (map[string]*virtv2.VirtualImage, error)
	ClusterVirtualImagesByName(ctx context.Context) (map[string]*virtv2.ClusterVirtualImage, error)
	VirtualMachineBlockDeviceAttachments(ctx context.Context) (map[virtv2.VMBDAObjectRef][]*virtv2.VirtualMachineBlockDeviceAttachment, error)
	IPAddress(ctx context.Context) (*virtv2.VirtualMachineIPAddress, error)
	VirtualMachineMACAddresses(ctx context.Context) ([]*virtv2.VirtualMachineMACAddress, error)
	Class(ctx context.Context) (*virtv2.VirtualMachineClass, error)
	VMOPs(ctx context.Context) ([]*virtv2.VirtualMachineOperation, error)
	Shared(fn func(s *Shared))
	ReadWriteOnceVirtualDisks(ctx context.Context) ([]*virtv2.VirtualDisk, error)
}

func New(c client.Client, vm *reconciler.Resource[*virtv2.VirtualMachine, virtv2.VirtualMachineStatus]) VirtualMachineState {
	return &state{client: c, vm: vm}
}

type state struct {
	client      client.Client
	mu          sync.RWMutex
	vm          *reconciler.Resource[*virtv2.VirtualMachine, virtv2.VirtualMachineStatus]
	pods        *corev1.PodList
	pod         *corev1.Pod
	vdByName    map[string]*virtv2.VirtualDisk
	viByName    map[string]*virtv2.VirtualImage
	cviByName   map[string]*virtv2.ClusterVirtualImage
	vmbdasByRef map[virtv2.VMBDAObjectRef][]*virtv2.VirtualMachineBlockDeviceAttachment
	ipAddress   *virtv2.VirtualMachineIPAddress
	vmmacs      []*virtv2.VirtualMachineMACAddress
	vmClass     *virtv2.VirtualMachineClass
	shared      Shared
}

type Shared struct {
	ShutdownInfo powerstate.ShutdownInfo
}

func (s *state) Shared(fn func(s *Shared)) {
	fn(&s.shared)
}

func (s *state) VirtualMachine() *reconciler.Resource[*virtv2.VirtualMachine, virtv2.VirtualMachineStatus] {
	return s.vm
}

func (s *state) KVVM(ctx context.Context) (*virtv1.VirtualMachine, error) {
	kvvm, err := object.FetchObject(ctx, s.vm.Name(), s.client, &virtv1.VirtualMachine{})
	if err != nil {
		return nil, fmt.Errorf("failed to fetch KVVM: %w", err)
	}
	return kvvm, nil
}

func (s *state) KVVMI(ctx context.Context) (*virtv1.VirtualMachineInstance, error) {
	kvvmi, err := object.FetchObject(ctx, s.vm.Name(), s.client, &virtv1.VirtualMachineInstance{})
	if err != nil {
		return nil, fmt.Errorf("failed to fetch KVVMI: %w", err)
	}
	return kvvmi, nil
}

func (s *state) Pods(ctx context.Context) (*corev1.PodList, error) {
	if s.vm == nil {
		return nil, nil
	}
	if s.pods != nil {
		return s.pods, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	podList := corev1.PodList{}
	err := s.client.List(ctx, &podList, &client.ListOptions{
		Namespace:     s.vm.Current().GetNamespace(),
		LabelSelector: labels.SelectorFromSet(map[string]string{virtv1.VirtualMachineNameLabel: s.vm.Current().GetName()}),
	})
	if err != nil && !k8serrors.IsNotFound(err) {
		return nil, fmt.Errorf("unable to list virt-launcher Pod for KubeVirt VM %q: %w", s.vm.Current().GetName(), err)
	}
	s.pods = &podList
	return s.pods, nil
}

func (s *state) Pod(ctx context.Context) (*corev1.Pod, error) {
	if s.vm == nil {
		return nil, nil
	}
	if s.pod != nil {
		return s.pod, nil
	}
	pods, err := s.Pods(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch pod for VirtualMachine %q: %w", s.vm.Current().GetName(), err)
	}
	kvvmi, err := s.KVVMI(ctx)
	if err != nil {
		return nil, err
	}
	var pod *corev1.Pod
	if len(pods.Items) > 0 {
		pod = kvvmutil.GetVMPod(kvvmi, pods)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.pod = pod
	return pod, nil
}

func (s *state) VirtualMachineBlockDeviceAttachments(ctx context.Context) (map[virtv2.VMBDAObjectRef][]*virtv2.VirtualMachineBlockDeviceAttachment, error) {
	if s.vm == nil {
		return nil, nil
	}
	if len(s.vmbdasByRef) > 0 {
		return s.vmbdasByRef, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	var vmbdas virtv2.VirtualMachineBlockDeviceAttachmentList
	err := s.client.List(ctx, &vmbdas, &client.ListOptions{
		Namespace: s.vm.Name().Namespace,
	})
	if err != nil {
		return nil, err
	}

	vmbdasByRef := make(map[virtv2.VMBDAObjectRef][]*virtv2.VirtualMachineBlockDeviceAttachment)
	for _, vmbda := range vmbdas.Items {
		if vmbda.Spec.VirtualMachineName != s.vm.Name().Name {
			continue
		}

		key := virtv2.VMBDAObjectRef{
			Kind: vmbda.Spec.BlockDeviceRef.Kind,
			Name: vmbda.Spec.BlockDeviceRef.Name,
		}

		vmbdasByRef[key] = append(vmbdasByRef[key], &vmbda)
	}

	s.vmbdasByRef = vmbdasByRef
	return vmbdasByRef, nil
}

func (s *state) VirtualDisk(ctx context.Context, name string) (*virtv2.VirtualDisk, error) {
	return object.FetchObject(ctx, types.NamespacedName{
		Name:      name,
		Namespace: s.vm.Current().GetNamespace(),
	}, s.client, &virtv2.VirtualDisk{})
}

func (s *state) VirtualImage(ctx context.Context, name string) (*virtv2.VirtualImage, error) {
	return object.FetchObject(ctx, types.NamespacedName{
		Name:      name,
		Namespace: s.vm.Current().GetNamespace(),
	}, s.client, &virtv2.VirtualImage{})
}

func (s *state) ClusterVirtualImage(ctx context.Context, name string) (*virtv2.ClusterVirtualImage, error) {
	return object.FetchObject(ctx, types.NamespacedName{
		Name: name,
	}, s.client, &virtv2.ClusterVirtualImage{})
}

func (s *state) VirtualDisksByName(ctx context.Context) (map[string]*virtv2.VirtualDisk, error) {
	if s.vm == nil {
		return nil, nil
	}
	if len(s.vdByName) > 0 {
		return s.vdByName, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	vdByName := make(map[string]*virtv2.VirtualDisk)
	for _, bd := range s.vm.Current().Spec.BlockDeviceRefs {
		switch bd.Kind {
		case virtv2.DiskDevice:
			vd, err := object.FetchObject(ctx, types.NamespacedName{
				Name:      bd.Name,
				Namespace: s.vm.Current().GetNamespace(),
			}, s.client, &virtv2.VirtualDisk{})
			if err != nil {
				return nil, fmt.Errorf("unable to get virtual disk %q: %w", bd.Name, err)
			}
			if vd == nil {
				continue
			}
			vdByName[bd.Name] = vd
		default:
			continue
		}
	}
	s.vdByName = vdByName
	return vdByName, nil
}

func (s *state) VirtualImagesByName(ctx context.Context) (map[string]*virtv2.VirtualImage, error) {
	if s.vm == nil {
		return nil, nil
	}
	if len(s.viByName) > 0 {
		return s.viByName, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	viByName := make(map[string]*virtv2.VirtualImage)
	for _, bd := range s.vm.Current().Spec.BlockDeviceRefs {
		switch bd.Kind {
		case virtv2.ImageDevice:
			vi, err := object.FetchObject(ctx, types.NamespacedName{
				Name:      bd.Name,
				Namespace: s.vm.Current().GetNamespace(),
			}, s.client, &virtv2.VirtualImage{})
			if err != nil {
				return nil, fmt.Errorf("unable to get VI %q: %w", bd.Name, err)
			}
			if vi == nil {
				continue
			}
			viByName[bd.Name] = vi
		default:
			continue
		}
	}
	s.viByName = viByName
	return viByName, nil
}

func (s *state) ClusterVirtualImagesByName(ctx context.Context) (map[string]*virtv2.ClusterVirtualImage, error) {
	if s.vm == nil {
		return nil, nil
	}
	if len(s.cviByName) > 0 {
		return s.cviByName, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	cviByName := make(map[string]*virtv2.ClusterVirtualImage)
	for _, bd := range s.vm.Current().Spec.BlockDeviceRefs {
		switch bd.Kind {
		case virtv2.ClusterImageDevice:
			cvi, err := object.FetchObject(ctx, types.NamespacedName{
				Name:      bd.Name,
				Namespace: s.vm.Current().GetNamespace(),
			}, s.client, &virtv2.ClusterVirtualImage{})
			if err != nil {
				return nil, fmt.Errorf("unable to get CVI %q: %w", bd.Name, err)
			}
			if cvi == nil {
				continue
			}
			cviByName[bd.Name] = cvi
		default:
			continue
		}
	}
	s.cviByName = cviByName
	return cviByName, nil
}

func (s *state) VirtualMachineMACAddresses(ctx context.Context) ([]*virtv2.VirtualMachineMACAddress, error) {
	if s.vm == nil {
		return nil, nil
	}

	if s.vmmacs != nil {
		return s.vmmacs, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	var vmmacs []*virtv2.VirtualMachineMACAddress
	for _, ns := range s.vm.Current().Spec.Networks {
		vmmacKey := types.NamespacedName{Name: ns.VirtualMachineMACAddressName, Namespace: s.vm.Current().GetNamespace()}
		vmmac, err := object.FetchObject(ctx, vmmacKey, s.client, &virtv2.VirtualMachineMACAddress{})
		if err != nil {
			return nil, fmt.Errorf("failed to fetch VirtualMachineMACAddress: %w", err)
		}
		if vmmac != nil {
			vmmacs = append(vmmacs, vmmac)
		}
	}

	vmmacList := &virtv2.VirtualMachineMACAddressList{}
	err := s.client.List(ctx, vmmacList, &client.ListOptions{
		Namespace:     s.vm.Current().GetNamespace(),
		LabelSelector: labels.SelectorFromSet(map[string]string{annotations.LabelVirtualMachineUID: string(s.vm.Current().GetUID())}),
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list VirtualMachineMACAddress: %w", err)
	}

	for _, vmmac := range vmmacList.Items {
		vmmacs = append(vmmacs, &vmmac)
	}

	s.vmmacs = vmmacs
	return s.vmmacs, nil
}

func (s *state) IPAddress(ctx context.Context) (*virtv2.VirtualMachineIPAddress, error) {
	if s.vm == nil {
		return nil, nil
	}

	if s.ipAddress != nil {
		return s.ipAddress, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	vmipName := s.vm.Current().Spec.VirtualMachineIPAddress
	if vmipName == "" {
		vmipList := &virtv2.VirtualMachineIPAddressList{}

		err := s.client.List(ctx, vmipList, &client.ListOptions{
			Namespace:     s.vm.Current().GetNamespace(),
			LabelSelector: labels.SelectorFromSet(map[string]string{annotations.LabelVirtualMachineUID: string(s.vm.Current().GetUID())}),
		})
		if err != nil {
			return nil, fmt.Errorf("failed to list VirtualMachineIPAddress: %w", err)
		}

		if len(vmipList.Items) == 0 {
			// TODO add search for resource by owner ref
			return nil, nil
		}

		s.ipAddress = &vmipList.Items[0]
	} else {
		vmipKey := types.NamespacedName{Name: vmipName, Namespace: s.vm.Current().GetNamespace()}

		ipAddress, err := object.FetchObject(ctx, vmipKey, s.client, &virtv2.VirtualMachineIPAddress{})
		if err != nil {
			return nil, fmt.Errorf("failed to fetch VirtualMachineIPAddress: %w", err)
		}
		s.ipAddress = ipAddress
	}

	return s.ipAddress, nil
}

func (s *state) Class(ctx context.Context) (*virtv2.VirtualMachineClass, error) {
	if s.vm == nil {
		return nil, nil
	}
	if s.vmClass != nil {
		return s.vmClass, nil
	}
	className := s.vm.Current().Spec.VirtualMachineClassName
	classKey := types.NamespacedName{Name: className}
	class, err := object.FetchObject(ctx, classKey, s.client, &virtv2.VirtualMachineClass{})
	if err != nil {
		return nil, fmt.Errorf("failed to fetch VirtualMachineClass: %w", err)
	}
	s.vmClass = class
	return s.vmClass, nil
}

func (s *state) VMOPs(ctx context.Context) ([]*virtv2.VirtualMachineOperation, error) {
	if s.vm == nil {
		return nil, nil
	}

	vm := s.vm.Current()
	vmops := &virtv2.VirtualMachineOperationList{}
	err := s.client.List(ctx, vmops, client.InNamespace(vm.Namespace))
	if err != nil {
		return nil, fmt.Errorf("failed to list VirtualMachineOperation: %w", err)
	}

	var resultVMOPs []*virtv2.VirtualMachineOperation

	for _, vmop := range vmops.Items {
		if vmop.Spec.VirtualMachine == vm.Name {
			resultVMOPs = append(resultVMOPs, &vmop)
		}
	}

	return resultVMOPs, nil
}

func (s *state) ReadWriteOnceVirtualDisks(ctx context.Context) ([]*virtv2.VirtualDisk, error) {
	vdByName, err := s.VirtualDisksByName(ctx)
	if err != nil {
		return nil, err
	}

	var nonMigratableVirtualDisks []*virtv2.VirtualDisk

	for _, vd := range vdByName {
		pvcKey := types.NamespacedName{Name: vd.Status.Target.PersistentVolumeClaim, Namespace: vd.Namespace}
		pvc, err := object.FetchObject(ctx, pvcKey, s.client, &corev1.PersistentVolumeClaim{})
		if err != nil {
			return nil, fmt.Errorf("failed to fetch PersistentVolumeClaim: %w", err)
		}
		if pvc == nil {
			nonMigratableVirtualDisks = append(nonMigratableVirtualDisks, vd)
			continue
		}

		rwx := false
		for _, mode := range pvc.Spec.AccessModes {
			if mode == corev1.ReadWriteMany {
				rwx = true
				break
			}
		}
		if !rwx {
			nonMigratableVirtualDisks = append(nonMigratableVirtualDisks, vd)
		}
	}

	return nonMigratableVirtualDisks, nil
}
