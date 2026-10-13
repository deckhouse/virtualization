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

package step

import (
	"context"
	"errors"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/common/ip"
	"github.com/deckhouse/virtualization-controller/pkg/common/network"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	intsvc "github.com/deckhouse/virtualization-controller/pkg/controller/vmip/internal/service"
	"github.com/deckhouse/virtualization-controller/pkg/eventrecord"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmipcondition"
)

type Allocator interface {
	GetAllocatedIPs(ctx context.Context) (ip.AllocatedIPs, error)
	AllocateNewIP(allocatedIPs ip.AllocatedIPs) (string, error)
	IsInsideOfRange(address string) error
}

type CreateLeaseStep struct {
	lease     *v1alpha2.VirtualMachineIPAddressLease
	allocator Allocator
	client    client.Client
	cb        *conditions.ConditionBuilder
	recorder  eventrecord.EventRecorderLogger
}

func NewCreateLeaseStep(
	lease *v1alpha2.VirtualMachineIPAddressLease,
	allocator Allocator,
	client client.Client,
	cb *conditions.ConditionBuilder,
	recorder eventrecord.EventRecorderLogger,
) *CreateLeaseStep {
	return &CreateLeaseStep{
		lease:     lease,
		allocator: allocator,
		client:    client,
		cb:        cb,
		recorder:  recorder,
	}
}

func (s CreateLeaseStep) Take(ctx context.Context, vmip *v1alpha2.VirtualMachineIPAddress) (*reconcile.Result, error) {
	if s.lease != nil {
		err := fmt.Errorf("the VirtualMachineIPAddressLease %q already exists, no need to create a new one, please report this as a bug", vmip.Name)
		s.cb.
			Status(metav1.ConditionFalse).
			Reason(vmipcondition.VirtualMachineIPAddressLeaseNotReady).
			Message(service.CapitalizeFirstLetter(err.Error()) + ".")
		return nil, err
	}

	// 1. Check if IP address has been already allocated but lost.
	if vmip.Status.Address != "" {
		s.cb.
			Status(metav1.ConditionFalse).
			Reason(vmipcondition.VirtualMachineIPAddressLeaseLost).
			Message(fmt.Sprintf("The VirtualMachineIPAddressLease %q doesn't exist.", ip.IPToLeaseName(vmip.Status.Address)))
		s.recorder.Event(vmip, corev1.EventTypeWarning, v1alpha2.ReasonFailed, fmt.Sprintf("The VirtualMachineIPAddressLease %q is lost.", ip.IPToLeaseName(vmip.Status.Address)))
		return &reconcile.Result{}, nil
	}

	if vmip.Spec.Type == v1alpha2.VirtualMachineIPAddressTypeAuto {
		mainIPAMAvailable, err := network.MainIPAMAvailable(ctx, s.client)
		if err != nil {
			return nil, fmt.Errorf("check whether the sdn module serves the Main network: %w", err)
		}
		if mainIPAMAvailable {
			return s.createLeaseForAddressFromSDN(ctx, vmip)
		}
	}

	allocatedIPs, err := s.allocator.GetAllocatedIPs(ctx)
	if err != nil {
		err = fmt.Errorf("failed to get allcoated IP addresses: %w", err)
		s.cb.
			Status(metav1.ConditionFalse).
			Reason(vmipcondition.VirtualMachineIPAddressLeaseNotReady).
			Message(service.CapitalizeFirstLetter(err.Error()) + ".")
		return nil, err
	}

	// 2. Allocate a new IP address or use the IP address provided in the spec.
	var ipAddress string
	if vmip.Spec.Type == v1alpha2.VirtualMachineIPAddressTypeStatic {
		ipAddress = vmip.Spec.StaticIP
	} else {
		ipAddress, err = s.allocator.AllocateNewIP(allocatedIPs)
		if err != nil {
			err = fmt.Errorf("failed to allocate new IP address: %w", err)
			s.cb.
				Status(metav1.ConditionFalse).
				Reason(vmipcondition.VirtualMachineIPAddressLeaseNotReady).
				Message(service.CapitalizeFirstLetter(err.Error()) + ".")
			return nil, err
		}
	}

	// 3. Verify that the allocated address does not exceed the permissible range.
	err = s.allocator.IsInsideOfRange(ipAddress)
	if err != nil {
		if errors.Is(err, intsvc.ErrIPAddressOutOfRange) {
			msg := fmt.Sprintf("The IP address %q is out of the valid range.", vmip.Spec.StaticIP)
			s.cb.
				Status(metav1.ConditionFalse).
				Reason(vmipcondition.VirtualMachineIPAddressIsOutOfTheValidRange).
				Message(msg)
			s.recorder.Event(vmip, corev1.EventTypeWarning, v1alpha2.ReasonFailed, msg)
			return &reconcile.Result{}, nil
		}

		err = fmt.Errorf("failed to check availability of IP address: %w", err)
		s.cb.
			Status(metav1.ConditionFalse).
			Reason(vmipcondition.VirtualMachineIPAddressLeaseNotReady).
			Message(service.CapitalizeFirstLetter(err.Error()) + ".")
		return &reconcile.Result{}, err
	}

	// 4. Ensure that the allocated IP address is not represented in the cluster by any lease.
	if _, ok := allocatedIPs[ipAddress]; ok {
		msg := fmt.Sprintf("The IP address %q belongs to existing VirtualMachineIPAddressLease and cannot be taken.", ipAddress)
		s.cb.
			Status(metav1.ConditionFalse).
			Reason(vmipcondition.VirtualMachineIPAddressLeaseAlreadyExists).
			Message(msg)
		s.recorder.Event(vmip, corev1.EventTypeWarning, v1alpha2.ReasonBound, msg)
		return &reconcile.Result{}, nil
	}

	// 5. All checks have passed, create a new lease.
	return s.createLease(ctx, vmip, ipAddress)
}

// createLeaseForAddressFromSDN creates the Auto mirror first and the lease on the
// address the sdn module gives it.
func (s CreateLeaseStep) createLeaseForAddressFromSDN(ctx context.Context, vmip *v1alpha2.VirtualMachineIPAddress) (*reconcile.Result, error) {
	// The protection handler has removed the mirror; nothing would delete a new one.
	if vmip.GetDeletionTimestamp() != nil {
		return &reconcile.Result{}, nil
	}

	vm, err := network.GetVMOfVMIP(ctx, s.client, vmip)
	if err != nil {
		return nil, err
	}
	if err := network.ReleaseTerminatingMirror(ctx, s.client, vmip, vm); err != nil {
		return nil, err
	}
	mirror, err := network.EnsureAutoMirror(ctx, s.client, vmip, network.MirrorOwnerRef(vmip, vm, false))
	switch {
	case err != nil:
		return setConditionFromMirrorError(s.cb, err)
	case mirror == nil:
		return setMirrorNameTakenCondition(s.cb, vmip.Name), nil
	case network.IPAddressPoolExhausted(mirror):
		setLeaseNotReadyCondition(s.cb, "No free IP addresses left in the pool of the Main network.")
		return &reconcile.Result{}, nil
	case !network.IPAddressAllocated(mirror) || mirror.Status.Address == "":
		setLeaseNotReadyCondition(s.cb, fmt.Sprintf("Waiting for the sdn module to allocate an address to the IPAddress %q.", mirror.GetName()))
		return &reconcile.Result{}, nil
	}

	// The sdn module does not see the leases of vmips without a mirror and may give
	// out such an address: the mirror is deleted, and the next one gets another.
	address := mirror.Status.Address
	lease := &v1alpha2.VirtualMachineIPAddressLease{}
	err = s.client.Get(ctx, types.NamespacedName{Name: ip.IPToLeaseName(address)}, lease)
	switch {
	case err == nil:
		if ref := lease.Spec.VirtualMachineIPAddressRef; ref != nil && ref.Namespace == vmip.Namespace && ref.Name == vmip.Name {
			// Our own lease from a reconcile the cache has not caught up with.
			return &reconcile.Result{}, nil
		}
		if err := network.DeleteSDNIPAddressByName(ctx, s.client, vmip.Namespace, mirror.GetName()); err != nil {
			return nil, err
		}
		setLeaseNotReadyCondition(s.cb, fmt.Sprintf("The IP address %q is already in use by another VirtualMachineIPAddress: picking another one.", address))
		return &reconcile.Result{}, nil
	case !k8serrors.IsNotFound(err):
		return nil, fmt.Errorf("get the VirtualMachineIPAddressLease of %s: %w", address, err)
	}

	return s.createLease(ctx, vmip, address)
}

func (s CreateLeaseStep) createLease(ctx context.Context, vmip *v1alpha2.VirtualMachineIPAddress, ipAddress string) (*reconcile.Result, error) {
	logger.FromContext(ctx).Info("Create lease", "ipAddress", ipAddress)

	err := s.client.Create(ctx, buildVirtualMachineIPAddressLease(vmip, ipAddress))
	switch {
	case err == nil:
		msg := fmt.Sprintf("The VirtualMachineIPAddressLease for the ip address %q has been created.", ipAddress)
		s.cb.
			Status(metav1.ConditionFalse).
			Reason(vmipcondition.VirtualMachineIPAddressLeaseNotReady).
			Message(msg)
		s.recorder.Event(vmip, corev1.EventTypeNormal, v1alpha2.ReasonBound, msg)
		return &reconcile.Result{}, nil
	case k8serrors.IsAlreadyExists(err):
		// The cache is outdated and not keeping up with the state in the cluster.
		// Wait for 2 seconds for the cache to update and try again.
		logger.FromContext(ctx).Warn("Lease already exists: requeue in 2s", "ipAddress", ipAddress)
		s.cb.
			Status(metav1.ConditionFalse).
			Reason(vmipcondition.VirtualMachineIPAddressLeaseNotFound).
			Message("Waiting for the IP address to be allocated and a new VirtualMachineIPAddressLease to be created.")
		return &reconcile.Result{RequeueAfter: 2 * time.Second}, nil
	default:
		err = fmt.Errorf("failed to create a new VirtualMachineIPAddressLease: %w", err)
		s.cb.
			Status(metav1.ConditionFalse).
			Reason(vmipcondition.VirtualMachineIPAddressLeaseNotFound).
			Message(service.CapitalizeFirstLetter(err.Error()) + ".")
		return nil, err
	}
}

func buildVirtualMachineIPAddressLease(vmip *v1alpha2.VirtualMachineIPAddress, ipAddress string) *v1alpha2.VirtualMachineIPAddressLease {
	return &v1alpha2.VirtualMachineIPAddressLease{
		ObjectMeta: metav1.ObjectMeta{
			Labels: map[string]string{
				annotations.LabelVirtualMachineIPAddressUID: string(vmip.GetUID()),
			},
			Name: ip.IPToLeaseName(ipAddress),
		},
		Spec: v1alpha2.VirtualMachineIPAddressLeaseSpec{
			VirtualMachineIPAddressRef: &v1alpha2.VirtualMachineIPAddressLeaseIpAddressRef{
				Name:      vmip.Name,
				Namespace: vmip.Namespace,
			},
		},
	}
}
