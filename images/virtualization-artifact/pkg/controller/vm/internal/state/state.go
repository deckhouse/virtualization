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
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type VirtualMachineState interface {
	VirtualMachine() *reconciler.Resource[*v1alpha2.VirtualMachine, v1alpha2.VirtualMachineStatus]
	KVVM(ctx context.Context) (*virtv1.VirtualMachine, error)
	KVVMI(ctx context.Context) (*virtv1.VirtualMachineInstance, error)
	Pods(ctx context.Context) (*corev1.PodList, error)
	Pod(ctx context.Context) (*corev1.Pod, error)
	VirtualDisk(ctx context.Context, name string) (*v1alpha2.VirtualDisk, error)
	VirtualImage(ctx context.Context, name string) (*v1alpha2.VirtualImage, error)
	ClusterVirtualImage(ctx context.Context, name string) (*v1alpha2.ClusterVirtualImage, error)
	VirtualDisksByName(ctx context.Context) (map[string]*v1alpha2.VirtualDisk, error)
	VirtualImagesByName(ctx context.Context) (map[string]*v1alpha2.VirtualImage, error)
	ClusterVirtualImagesByName(ctx context.Context) (map[string]*v1alpha2.ClusterVirtualImage, error)
	VirtualMachineBlockDeviceAttachments(ctx context.Context) (map[v1alpha2.VMBDAObjectRef][]*v1alpha2.VirtualMachineBlockDeviceAttachment, error)
	IPAddress(ctx context.Context) (*v1alpha2.VirtualMachineIPAddress, error)
	VirtualMachineMACAddresses(ctx context.Context) ([]*v1alpha2.VirtualMachineMACAddress, error)
	Class(ctx context.Context) (*v1alpha2.VirtualMachineClass, error)
	VMOPs(ctx context.Context) ([]*v1alpha2.VirtualMachineOperation, error)
	Shared(fn func(s *Shared))
	ReadWriteOnceVirtualDisks(ctx context.Context) ([]*v1alpha2.VirtualDisk, error)
}

func New(c client.Client, vm *reconciler.Resource[*v1alpha2.VirtualMachine, v1alpha2.VirtualMachineStatus]) VirtualMachineState {
	state := &state{client: c, vm: vm}
	state.fill()
	return state
}

type Shared struct {
	ShutdownInfo powerstate.ShutdownInfo
}

type state struct {
	client client.Client
	vm     *reconciler.Resource[*v1alpha2.VirtualMachine, v1alpha2.VirtualMachineStatus]
	shared Shared
	bdRefs []blockDeviceRef
}

type blockDeviceRef struct {
	Name string
	Kind v1alpha2.BlockDeviceKind
}

func (s *state) fill() {
	mapRefs := make(map[blockDeviceRef]struct{})

	for _, bd := range s.vm.Current().Spec.BlockDeviceRefs {
		mapRefs[blockDeviceRef{Name: bd.Name, Kind: bd.Kind}] = struct{}{}
	}
	for _, bd := range s.vm.Current().Status.BlockDeviceRefs {
		mapRefs[blockDeviceRef{Name: bd.Name, Kind: bd.Kind}] = struct{}{}
	}

	s.bdRefs = make([]blockDeviceRef, 0, len(mapRefs))
	for ref := range mapRefs {
		s.bdRefs = append(s.bdRefs, ref)
	}
}

func (s *state) Shared(fn func(s *Shared)) {
	fn(&s.shared)
}

func (s *state) VirtualMachine() *reconciler.Resource[*v1alpha2.VirtualMachine, v1alpha2.VirtualMachineStatus] {
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
	podList := corev1.PodList{}
	err := s.client.List(ctx, &podList, &client.ListOptions{
		Namespace:     s.vm.Current().GetNamespace(),
		LabelSelector: labels.SelectorFromSet(map[string]string{virtv1.VirtualMachineNameLabel: s.vm.Current().GetName()}),
	})
	if err != nil && !k8serrors.IsNotFound(err) {
		return nil, fmt.Errorf("unable to list virt-launcher Pod for KubeVirt VM %q: %w", s.vm.Current().GetName(), err)
	}
	return &podList, nil
}

func (s *state) Pod(ctx context.Context) (*corev1.Pod, error) {
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
	return pod, nil
}

func (s *state) VirtualMachineBlockDeviceAttachments(ctx context.Context) (map[v1alpha2.VMBDAObjectRef][]*v1alpha2.VirtualMachineBlockDeviceAttachment, error) {
	var vmbdas v1alpha2.VirtualMachineBlockDeviceAttachmentList
	err := s.client.List(ctx, &vmbdas, &client.ListOptions{
		Namespace: s.vm.Name().Namespace,
	})
	if err != nil {
		return nil, err
	}

	vmbdasByRef := make(map[v1alpha2.VMBDAObjectRef][]*v1alpha2.VirtualMachineBlockDeviceAttachment)
	for _, vmbda := range vmbdas.Items {
		if vmbda.Spec.VirtualMachineName != s.vm.Name().Name {
			continue
		}

		key := v1alpha2.VMBDAObjectRef{
			Kind: vmbda.Spec.BlockDeviceRef.Kind,
			Name: vmbda.Spec.BlockDeviceRef.Name,
		}

		vmbdasByRef[key] = append(vmbdasByRef[key], &vmbda)
	}

	return vmbdasByRef, nil
}

func (s *state) VirtualDisk(ctx context.Context, name string) (*v1alpha2.VirtualDisk, error) {
	return object.FetchObject(ctx, types.NamespacedName{
		Name:      name,
		Namespace: s.vm.Current().GetNamespace(),
	}, s.client, &v1alpha2.VirtualDisk{})
}

func (s *state) VirtualImage(ctx context.Context, name string) (*v1alpha2.VirtualImage, error) {
	return object.FetchObject(ctx, types.NamespacedName{
		Name:      name,
		Namespace: s.vm.Current().GetNamespace(),
	}, s.client, &v1alpha2.VirtualImage{})
}

func (s *state) ClusterVirtualImage(ctx context.Context, name string) (*v1alpha2.ClusterVirtualImage, error) {
	return object.FetchObject(ctx, types.NamespacedName{
		Name: name,
	}, s.client, &v1alpha2.ClusterVirtualImage{})
}

func (s *state) VirtualDisksByName(ctx context.Context) (map[string]*v1alpha2.VirtualDisk, error) {
	vdByName := make(map[string]*v1alpha2.VirtualDisk)
	for _, bd := range s.bdRefs {
		switch bd.Kind {
		case v1alpha2.DiskDevice:
			vd, err := object.FetchObject(ctx, types.NamespacedName{
				Name:      bd.Name,
				Namespace: s.vm.Current().GetNamespace(),
			}, s.client, &v1alpha2.VirtualDisk{})
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
	return vdByName, nil
}

func (s *state) VirtualImagesByName(ctx context.Context) (map[string]*v1alpha2.VirtualImage, error) {
	viByName := make(map[string]*v1alpha2.VirtualImage)
	for _, bd := range s.bdRefs {
		switch bd.Kind {
		case v1alpha2.ImageDevice:
			vi, err := object.FetchObject(ctx, types.NamespacedName{
				Name:      bd.Name,
				Namespace: s.vm.Current().GetNamespace(),
			}, s.client, &v1alpha2.VirtualImage{})
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
	return viByName, nil
}

func (s *state) ClusterVirtualImagesByName(ctx context.Context) (map[string]*v1alpha2.ClusterVirtualImage, error) {
	cviByName := make(map[string]*v1alpha2.ClusterVirtualImage)
	for _, bd := range s.bdRefs {
		switch bd.Kind {
		case v1alpha2.ClusterImageDevice:
			cvi, err := object.FetchObject(ctx, types.NamespacedName{
				Name:      bd.Name,
				Namespace: s.vm.Current().GetNamespace(),
			}, s.client, &v1alpha2.ClusterVirtualImage{})
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
	return cviByName, nil
}

func (s *state) VirtualMachineMACAddresses(ctx context.Context) ([]*v1alpha2.VirtualMachineMACAddress, error) {
	var vmmacs []*v1alpha2.VirtualMachineMACAddress
	for _, ns := range s.vm.Current().Spec.Networks {
		vmmacKey := types.NamespacedName{Name: ns.VirtualMachineMACAddressName, Namespace: s.vm.Current().GetNamespace()}
		vmmac, err := object.FetchObject(ctx, vmmacKey, s.client, &v1alpha2.VirtualMachineMACAddress{})
		if err != nil {
			return nil, fmt.Errorf("failed to fetch VirtualMachineMACAddress: %w", err)
		}
		if vmmac != nil {
			vmmacs = append(vmmacs, vmmac)
		}
	}

	vmmacList := &v1alpha2.VirtualMachineMACAddressList{}
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

	return vmmacs, nil
}

func (s *state) IPAddress(ctx context.Context) (*v1alpha2.VirtualMachineIPAddress, error) {
	vmipName := s.vm.Current().Spec.VirtualMachineIPAddress
	if vmipName == "" {
		vmipList := &v1alpha2.VirtualMachineIPAddressList{}

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

		return &vmipList.Items[0], nil
	}

	vmipKey := types.NamespacedName{Name: vmipName, Namespace: s.vm.Current().GetNamespace()}

	ipAddress, err := object.FetchObject(ctx, vmipKey, s.client, &v1alpha2.VirtualMachineIPAddress{})
	if err != nil {
		return nil, fmt.Errorf("failed to fetch VirtualMachineIPAddress: %w", err)
	}

	return ipAddress, nil
}

func (s *state) Class(ctx context.Context) (*v1alpha2.VirtualMachineClass, error) {
	className := s.vm.Current().Spec.VirtualMachineClassName
	classKey := types.NamespacedName{Name: className}
	class, err := object.FetchObject(ctx, classKey, s.client, &v1alpha2.VirtualMachineClass{})
	if err != nil {
		return nil, fmt.Errorf("failed to fetch VirtualMachineClass: %w", err)
	}
	return class, nil
}

func (s *state) VMOPs(ctx context.Context) ([]*v1alpha2.VirtualMachineOperation, error) {
	vm := s.vm.Current()
	vmops := &v1alpha2.VirtualMachineOperationList{}
	err := s.client.List(ctx, vmops, client.InNamespace(vm.Namespace))
	if err != nil {
		return nil, fmt.Errorf("failed to list VirtualMachineOperation: %w", err)
	}

	var resultVMOPs []*v1alpha2.VirtualMachineOperation

	for _, vmop := range vmops.Items {
		if vmop.Spec.VirtualMachine == vm.Name {
			resultVMOPs = append(resultVMOPs, &vmop)
		}
	}

	return resultVMOPs, nil
}

func (s *state) ReadWriteOnceVirtualDisks(ctx context.Context) ([]*v1alpha2.VirtualDisk, error) {
	vdByName, err := s.VirtualDisksByName(ctx)
	if err != nil {
		return nil, err
	}

	var nonMigratableVirtualDisks []*v1alpha2.VirtualDisk

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
