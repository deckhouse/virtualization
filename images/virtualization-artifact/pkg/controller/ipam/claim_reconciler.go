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

package ipam

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	k8snet "k8s.io/utils/net"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/deckhouse/virtualization-controller/pkg/controller/common"
	"github.com/deckhouse/virtualization-controller/pkg/sdk/framework/two_phase_reconciler"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type ClaimReconciler struct {
	ParsedCIDRs []*net.IPNet
}

func NewClaimReconciler(virtualMachineCIDRs []string) (*ClaimReconciler, error) {
	parsedCIDRs := make([]*net.IPNet, len(virtualMachineCIDRs))

	for i, cidr := range virtualMachineCIDRs {
		_, parsedCIDR, err := net.ParseCIDR(cidr)
		if err != nil || parsedCIDR == nil {
			return nil, fmt.Errorf("failed to parse virtual cide %s: %w", cidr, err)
		}

		parsedCIDRs[i] = parsedCIDR
	}

	return &ClaimReconciler{
		ParsedCIDRs: parsedCIDRs,
	}, nil
}

func (r *ClaimReconciler) SetupController(_ context.Context, mgr manager.Manager, ctr controller.Controller) error {
	if err := ctr.Watch(
		source.Kind(mgr.GetCache(), &virtv2.VirtualMachineIPAddressLease{}),
		handler.EnqueueRequestsFromMapFunc(r.enqueueRequestsFromLeases),
		predicate.Funcs{
			CreateFunc: func(e event.CreateEvent) bool { return true },
			DeleteFunc: func(e event.DeleteEvent) bool { return true },
			UpdateFunc: func(e event.UpdateEvent) bool { return false },
		},
	); err != nil {
		return fmt.Errorf("error setting watch on leases: %w", err)
	}

	if err := ctr.Watch(
		source.Kind(mgr.GetCache(), &virtv2.VirtualMachine{}),
		handler.EnqueueRequestsFromMapFunc(r.enqueueRequestsFromVMs),
		predicate.Funcs{
			CreateFunc: func(e event.CreateEvent) bool { return false },
			DeleteFunc: func(e event.DeleteEvent) bool { return true },
			UpdateFunc: func(e event.UpdateEvent) bool { return true },
		},
	); err != nil {
		return fmt.Errorf("error setting watch on vms: %w", err)
	}

	return ctr.Watch(source.Kind(mgr.GetCache(), &virtv2.VirtualMachineIPAddressClaim{}), &handler.EnqueueRequestForObject{})
}

func (r *ClaimReconciler) enqueueRequestsFromVMs(_ context.Context, obj client.Object) []reconcile.Request {
	vm, ok := obj.(*virtv2.VirtualMachine)
	if !ok {
		return nil
	}

	if vm.Spec.VirtualMachineIPAddressClaim == "" {
		return []reconcile.Request{
			{
				NamespacedName: types.NamespacedName{
					Namespace: vm.Namespace,
					Name:      vm.Name,
				},
			},
		}
	}

	return []reconcile.Request{
		{
			NamespacedName: types.NamespacedName{
				Namespace: vm.Namespace,
				Name:      vm.Spec.VirtualMachineIPAddressClaim,
			},
		},
	}
}

func (r *ClaimReconciler) enqueueRequestsFromLeases(_ context.Context, obj client.Object) []reconcile.Request {
	lease, ok := obj.(*virtv2.VirtualMachineIPAddressLease)
	if !ok {
		return nil
	}

	if lease.Spec.ClaimRef == nil {
		return nil
	}

	return []reconcile.Request{
		{
			NamespacedName: types.NamespacedName{
				Namespace: lease.Spec.ClaimRef.Namespace,
				Name:      lease.Spec.ClaimRef.Name,
			},
		},
	}
}

func (r *ClaimReconciler) Sync(ctx context.Context, _ reconcile.Request, state *ClaimReconcilerState, opts two_phase_reconciler.ReconcilerOptions) error {
	switch {
	case shouldUnboundClaim(state):
		opts.Log.Info("The claim is no longer used by the VM: unbound")

		controllerutil.RemoveFinalizer(state.Claim.Changed(), virtv2.FinalizerIPAddressClaimCleanup)

		if state.Claim.Current().Labels[common.LabelImplicitIPAddressClaim] == common.LabelImplicitIPAddressClaimValue {
			opts.Log.Info("The claim is implicit: delete it")
			return opts.Client.Delete(ctx, state.Claim.Current())
		}
	case controllerutil.AddFinalizer(state.Claim.Changed(), virtv2.FinalizerIPAddressClaimCleanup):
		state.SetReconcilerResult(&reconcile.Result{Requeue: true})
		return nil
	}

	switch {
	case state.Lease == nil && state.Claim.Current().Spec.VirtualMachineIPAddressLease != "":
		opts.Log.Info("Lease by name not found: waiting for the lease to be available")
		return nil

	case state.Lease == nil:
		// Lease not found by spec.virtualMachineIPAddressLease or spec.Address: it doesn't exist.
		opts.Log.Info("No Lease for Claim: create the new one", "address", state.Claim.Current().Spec.Address, "leaseName", state.Claim.Current().Spec.VirtualMachineIPAddressLease)

		leaseName := state.Claim.Current().Spec.VirtualMachineIPAddressLease

		if state.Claim.Current().Spec.Address == "" {
			if leaseName != "" {
				opts.Log.Info("Claim address omitted in the spec: extract from the lease name")
				state.Claim.Changed().Spec.Address = leaseNameToIP(leaseName)
			} else {
				opts.Log.Info("Claim address omitted in the spec: allocate the new one")
				var err error
				state.Claim.Changed().Spec.Address, err = r.allocateNewIP(state.AllocatedIPs)
				if err != nil {
					return err
				}
			}
		}

		if !r.isAvailableAddress(state.Claim.Changed().Spec.Address, state.AllocatedIPs) {
			opts.Log.Info("Claim cannot be created: the address has already been allocated for another claim", "address", state.Claim.Current().Spec.Address)
			return nil
		}

		if leaseName == "" {
			leaseName = ipToLeaseName(state.Claim.Changed().Spec.Address)
		}

		opts.Log.Info("Create lease",
			"leaseName", leaseName,
			"reclaimPolicy", state.Claim.Current().Spec.ReclaimPolicy,
			"refName", state.Claim.Name().Name,
			"refNamespace", state.Claim.Name().Namespace,
		)

		state.Claim.Changed().Spec.VirtualMachineIPAddressLease = leaseName

		err := opts.Client.Update(ctx, state.Claim.Changed())
		if err != nil {
			return fmt.Errorf("failed to set lease name for claim: %w", err)
		}

		err = opts.Client.Create(ctx, &virtv2.VirtualMachineIPAddressLease{
			ObjectMeta: metav1.ObjectMeta{
				Name: leaseName,
			},
			Spec: virtv2.VirtualMachineIPAddressLeaseSpec{
				ReclaimPolicy: state.Claim.Current().Spec.ReclaimPolicy,
				ClaimRef: &virtv2.VirtualMachineIPAddressLeaseClaimRef{
					Name:      state.Claim.Name().Name,
					Namespace: state.Claim.Name().Namespace,
				},
			},
		})
		if err != nil {
			return err
		}

		return nil

	case state.Lease.Status.Phase == "":
		opts.Log.Info("Lease is not ready: waiting for the lease")
		state.SetReconcilerResult(&reconcile.Result{Requeue: true, RequeueAfter: 2 * time.Second})
		return nil

	case isBoundLease(state):
		opts.Log.Info("Lease already exists, claim ref is valid")
		return nil

	case state.Lease.Status.Phase == virtv2.VirtualMachineIPAddressLeasePhaseBound:
		opts.Log.Info("Lease is bounded to another claim: recreate claim when the lease is released")
		return nil

	default:
		opts.Log.Info("Lease is released: set binding")

		state.Lease.Spec.ReclaimPolicy = state.Claim.Current().Spec.ReclaimPolicy
		state.Lease.Spec.ClaimRef = &virtv2.VirtualMachineIPAddressLeaseClaimRef{
			Name:      state.Claim.Name().Name,
			Namespace: state.Claim.Name().Namespace,
		}

		err := opts.Client.Update(ctx, state.Lease)
		if err != nil {
			return err
		}

		state.Claim.Changed().Spec.VirtualMachineIPAddressLease = state.Lease.Name
		state.Claim.Changed().Spec.Address = leaseNameToIP(state.Lease.Name)

		return opts.Client.Update(ctx, state.Claim.Changed())
	}
}

func (r *ClaimReconciler) UpdateStatus(_ context.Context, _ reconcile.Request, state *ClaimReconcilerState, opts two_phase_reconciler.ReconcilerOptions) error {
	// Do nothing if object is being deleted as any update will lead to en error.
	if state.isDeletion() {
		return nil
	}

	claimStatus := state.Claim.Current().Status.DeepCopy()

	claimStatus.VirtualMachine = ""
	if state.VM != nil {
		claimStatus.VirtualMachine = state.VM.Name
	}

	claimStatus.Address = ""
	claimStatus.ConflictMessage = ""

	switch {
	case state.Lease == nil && state.Claim.Current().Spec.VirtualMachineIPAddressLease != "":
		claimStatus.Phase = virtv2.VirtualMachineIPAddressClaimPhaseLost

	case state.Lease == nil:
		claimStatus.Phase = virtv2.VirtualMachineIPAddressClaimPhasePending

	case isBoundLease(state):
		claimStatus.Phase = virtv2.VirtualMachineIPAddressClaimPhaseBound
		claimStatus.Address = state.Claim.Current().Spec.Address

	case state.Lease.Status.Phase == virtv2.VirtualMachineIPAddressLeasePhaseBound:
		claimStatus.Phase = virtv2.VirtualMachineIPAddressClaimPhaseConflict

		// There is only one way to automatically link Claim in phase Conflict with recently released Lease: only with cyclic reconciliation (with an interval of N seconds).
		// At the moment this looks redundant, so Claim in the phase Conflict will not be able to bind the recently released Lease.
		// It is necessary to recreate Claim manually in order to link it to released Lease.
		claimStatus.ConflictMessage = "Lease is bounded to another claim: please recreate claim when the lease is released"

	default:
		claimStatus.Phase = virtv2.VirtualMachineIPAddressClaimPhasePending
	}

	opts.Log.Info("Set claim phase", "phase", claimStatus.Phase)

	state.Claim.Changed().Status = *claimStatus

	return nil
}

func shouldUnboundClaim(state *ClaimReconcilerState) bool {
	// Claim is bound, but VM not found.
	return state.VM == nil
}

func isBoundLease(state *ClaimReconcilerState) bool {
	if state.Lease.Status.Phase != virtv2.VirtualMachineIPAddressLeasePhaseBound {
		return false
	}

	if state.Lease.Spec.ClaimRef == nil {
		return false
	}

	if state.Lease.Spec.ClaimRef.Namespace != state.Claim.Name().Namespace || state.Lease.Spec.ClaimRef.Name != state.Claim.Name().Name {
		return false
	}

	return true
}

func (r *ClaimReconciler) isAvailableAddress(address string, allocatedIPs AllocatedIPs) bool {
	ip := net.ParseIP(address)

	if _, ok := allocatedIPs[ip.String()]; !ok {
		for _, cidr := range r.ParsedCIDRs {
			if cidr.Contains(ip) {
				// available
				return true
			}
		}
		// out of range
		return false
	}
	// already exists
	return false
}

func (r *ClaimReconciler) allocateNewIP(allocatedIPs AllocatedIPs) (string, error) {
	for _, cidr := range r.ParsedCIDRs {
		for ip := cidr.IP.Mask(cidr.Mask); cidr.Contains(ip); inc(ip) {
			// Allow allocation of IP address explicitly specified using a 32-bit mask.
			if k8snet.RangeSize(cidr) != 1 {
				// Skip the allocation of the first or last addresses within the CIDR range.
				isFirstLast, err := isFirstLastIP(ip, cidr)
				if err != nil {
					return "", err
				}

				if isFirstLast {
					continue
				}
			}

			_, ok := allocatedIPs[ip.String()]
			if !ok {
				return ip.String(), nil
			}
		}
	}
	return "", errors.New("no remaining ips")
}

const ipPrefix = "ip-"

func ipToLeaseName(ip string) string {
	addr := net.ParseIP(ip)
	if addr.To4() != nil {
		// IPv4 address
		return ipPrefix + strings.ReplaceAll(addr.String(), ".", "-")
	}

	return ""
}

func leaseNameToIP(leaseName string) string {
	if strings.HasPrefix(leaseName, ipPrefix) && len(leaseName) > len(ipPrefix) {
		return strings.ReplaceAll(leaseName[len(ipPrefix):], "-", ".")
	}

	return ""
}

func isFirstLastIP(ip net.IP, cidr *net.IPNet) (bool, error) {
	size := int(k8snet.RangeSize(cidr))

	first, err := k8snet.GetIndexedIP(cidr, 0)
	if err != nil {
		return false, err
	}

	if first.Equal(ip) {
		return true, nil
	}

	last, err := k8snet.GetIndexedIP(cidr, size-1)
	if err != nil {
		return false, err
	}

	return last.Equal(ip), nil
}

// http://play.golang.org/p/m8TNTtygK0
func inc(ip net.IP) {
	for j := len(ip) - 1; j >= 0; j-- {
		ip[j]++
		if ip[j] > 0 {
			break
		}
	}
}
