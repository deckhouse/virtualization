/*
Copyright 2026 Flant JSC

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

// Package vmsop drives VirtualMachineSnapshotOperation{spec.type: CreateVirtualMachine} restore/clone
// through the recursively-compiled manifest tree (internal/restore), for operations whose source
// VirtualMachineSnapshot was captured by the unified-snapshotter SDK controller.
//
// Scope note (limited PoC, mode: Strict only): the existing CRD still requires (CEL) at least one
// nameReplacement or a customization, to avoid the restored VirtualMachine colliding by name with the
// still-existing source. This controller accepts exactly ONE nameReplacement entry — renaming the
// source VirtualMachine — and rejects spec.createVirtualMachine.customization and any other
// nameReplacement entries: unlike the built-in controller, it does not implement DryRun/BestEffort,
// arbitrary per-resource renaming, or name prefix/suffix customization. VirtualDisk (and every other
// restored resource) keeps its captured name unchanged, so this only works cleanly once the source
// VirtualMachine's disks no longer exist under those names (a genuine restore-after-deletion, or a
// clone where the caller has otherwise freed up the disk names) — Create returns AlreadyExists
// otherwise, and this controller surfaces that as a terminal failure rather than silently overwriting
// anything.
package vmsop

import (
	"context"
	"fmt"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/deckhouse/pkg/log"
	"github.com/deckhouse/virtualization-controller/pkg/controller/unified-snapshotter/internal/restore"
	"github.com/deckhouse/virtualization-controller/pkg/controller/unified-snapshotter/internal/statuspatch"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmsopcondition"
)

const (
	requeueAfter   = 3 * time.Second
	ControllerName = "virtualmachine-snapshot-operation-controller"
)

// annCreatedByVMSOP marks an object this controller created, keyed by the creating
// VirtualMachineSnapshotOperation's UID. It lets a retried reconcile (e.g. after a partial failure
// created the disks but not the VM) recognize its own prior output on AlreadyExists instead of treating
// it as a genuine name conflict.
const annCreatedByVMSOP = "virtualization.deckhouse.io/created-by-unified-snapshotter-vmsop"

// Reconciler drives VirtualMachineSnapshotOperation{type: CreateVirtualMachine, mode: Strict} restore.
type Reconciler struct {
	Client   client.Client
	Compiler *restore.Compiler
	Log      *log.Logger
}

// NewReconciler builds a Reconciler, including its state-snapshotter manifests-download client and
// restore compiler (internal/restore) — kept behind this constructor because that package is
// Go-internal to this controller family and so cannot be constructed from outside it (e.g. from
// cmd/virtualization-controller/main.go).
func NewReconciler(cfg *rest.Config, c client.Client, log *log.Logger) (*Reconciler, error) {
	manifestClient, err := restore.NewManifestClient(cfg)
	if err != nil {
		return nil, fmt.Errorf("build state-snapshotter manifests-download client: %w", err)
	}
	return &Reconciler{
		Client:   c,
		Compiler: restore.NewCompiler(c, manifestClient),
		Log:      log,
	}, nil
}

// SetupWithManager registers the reconciler against every VirtualMachineSnapshotOperation.
func (r *Reconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		Named(ControllerName).
		For(&v1alpha2.VirtualMachineSnapshotOperation{}).
		WithLogConstructor(logger.NewConstructor(r.Log)).
		Complete(r)
}

func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	vmsop := &v1alpha2.VirtualMachineSnapshotOperation{}
	if err := r.Client.Get(ctx, req.NamespacedName, vmsop); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	if vmsop.DeletionTimestamp != nil {
		return ctrl.Result{}, nil
	}
	if vmsop.Spec.Type != v1alpha2.VMSOPTypeCreateVirtualMachine {
		return ctrl.Result{}, nil
	}

	// spec is immutable and this operation is one-shot: a terminal phase never re-processes (matches
	// the SDK's own "capture is a point-in-time, never re-planned" invariant for the same reason —
	// re-running Create against already-created resources is not what an operator asking for a retry
	// wants).
	if vmsop.Status.Phase == v1alpha2.VMSOPPhaseCompleted || vmsop.Status.Phase == v1alpha2.VMSOPPhaseFailed {
		return ctrl.Result{}, nil
	}

	vms := &v1alpha2.VirtualMachineSnapshot{}
	if err := r.Client.Get(ctx, types.NamespacedName{Namespace: vmsop.Namespace, Name: vmsop.Spec.VirtualMachineSnapshotName}, vms); err != nil {
		if apierrors.IsNotFound(err) {
			return r.progress(ctx, vmsop, vmsopcondition.ReasonVirtualMachineSnapshotNotFound, fmt.Sprintf(
				"source VirtualMachineSnapshot %q not found; waiting", vmsop.Spec.VirtualMachineSnapshotName))
		}
		return ctrl.Result{}, err
	}
	if vms.Status.CaptureState == nil {
		// Not ours: the source VirtualMachineSnapshot was captured by the legacy controller.
		return ctrl.Result{}, nil
	}

	newVMName, failMsg := r.validateSpec(vmsop)
	if failMsg != "" {
		return r.fail(ctx, vmsop, failMsg)
	}
	rule := vmsop.Spec.CreateVirtualMachine.NameReplacement[0]
	if rule.From.Name != "" && rule.From.Name != vms.Spec.VirtualMachineName {
		return r.fail(ctx, vmsop, fmt.Sprintf(
			"nameReplacement.from.name %q does not match the snapshot's source VirtualMachine %q", rule.From.Name, vms.Spec.VirtualMachineName))
	}
	if vms.Status.Phase != v1alpha2.VirtualMachineSnapshotPhaseReady {
		return r.progress(ctx, vmsop, vmsopcondition.ReasonNotReadyToBeExecuted, fmt.Sprintf(
			"source VirtualMachineSnapshot %q is not Ready yet; waiting", vms.Name))
	}

	objs, err := r.Compiler.CompileVirtualMachineSnapshot(ctx, vms.Namespace, vms.Name, vmsop.Namespace)
	if err != nil {
		return r.progress(ctx, vmsop, vmsopcondition.ReasonNotReadyToBeExecuted, fmt.Sprintf("compiling restore manifests: %s", err))
	}
	if err := renameVirtualMachine(objs, vms.Spec.VirtualMachineName, newVMName); err != nil {
		return r.fail(ctx, vmsop, err.Error())
	}

	resources := make([]v1alpha2.SnapshotResourceStatus, 0, len(objs))
	for i := range objs {
		obj := objs[i]
		markCreatedBy(&obj, string(vmsop.UID))
		if err := r.Client.Create(ctx, &obj); err != nil {
			if apierrors.IsAlreadyExists(err) {
				existing := obj.DeepCopy()
				if getErr := r.Client.Get(ctx, types.NamespacedName{Namespace: obj.GetNamespace(), Name: obj.GetName()}, existing); getErr == nil &&
					existing.GetAnnotations()[annCreatedByVMSOP] == string(vmsop.UID) {
					// Our own output from a previous, partially-failed attempt at this same operation.
					resources = append(resources, resourceStatus(obj, v1alpha2.SnapshotResourceStatusCompleted, ""))
					continue
				}
				return r.fail(ctx, vmsop, fmt.Sprintf("%s %q already exists and was not created by this operation", obj.GetKind(), obj.GetName()))
			}
			resources = append(resources, resourceStatus(obj, v1alpha2.SnapshotResourceStatusFailed, err.Error()))
			vmsop.Status.Resources = resources
			_ = r.patchStatus(ctx, vmsop)
			return ctrl.Result{RequeueAfter: requeueAfter}, nil
		}
		resources = append(resources, resourceStatus(obj, v1alpha2.SnapshotResourceStatusCompleted, ""))
	}

	return r.complete(ctx, vmsop, resources)
}

// vmsopOwnedStatus lists exactly the VirtualMachineSnapshotOperationStatus fields this controller ever
// sets. Unlike VirtualDiskSnapshot/VirtualMachineSnapshot, no other controller writes to a
// VirtualMachineSnapshotOperation's status at all, so this is more a matter of staying consistent with
// the other two controllers than a real safety requirement — see internal/statuspatch.
type vmsopOwnedStatus struct {
	Phase      v1alpha2.VMSOPPhase               `json:"phase,omitempty"`
	Conditions []metav1.Condition                `json:"conditions,omitempty"`
	Resources  []v1alpha2.SnapshotResourceStatus `json:"resources,omitempty"`
}

func (r *Reconciler) patchStatus(ctx context.Context, vmsop *v1alpha2.VirtualMachineSnapshotOperation) error {
	patch, err := statuspatch.For(v1alpha2.SchemeGroupVersion.WithKind(v1alpha2.VirtualMachineSnapshotOperationKind), vmsopOwnedStatus{
		Phase:      vmsop.Status.Phase,
		Conditions: vmsop.Status.Conditions,
		Resources:  vmsop.Status.Resources,
	})
	if err != nil {
		return err
	}
	return r.Client.Status().Patch(ctx, vmsop, patch)
}

// validateSpec enforces this PoC controller's Strict-only, single-rename-only scope. It returns the
// new VirtualMachine name and an empty failMsg on success, or an empty name and a human-readable
// failMsg otherwise.
func (r *Reconciler) validateSpec(vmsop *v1alpha2.VirtualMachineSnapshotOperation) (newVMName, failMsg string) {
	create := vmsop.Spec.CreateVirtualMachine
	if create == nil {
		return "", "spec.createVirtualMachine is required"
	}
	if create.Mode != v1alpha2.SnapshotOperationModeStrict {
		return "", fmt.Sprintf("this unified-snapshotter PoC controller supports only mode: Strict (got %q)", create.Mode)
	}
	if create.Customization != nil {
		return "", "this unified-snapshotter PoC controller does not implement spec.createVirtualMachine.customization; only a single nameReplacement renaming the source VirtualMachine is supported"
	}
	if len(create.NameReplacement) != 1 {
		return "", fmt.Sprintf("this unified-snapshotter PoC controller supports exactly one nameReplacement entry (renaming the source VirtualMachine); got %d", len(create.NameReplacement))
	}
	rule := create.NameReplacement[0]
	if rule.From.Kind != "" && rule.From.Kind != v1alpha2.VirtualMachineKind {
		return "", fmt.Sprintf("the single supported nameReplacement entry must target kind %q, got %q", v1alpha2.VirtualMachineKind, rule.From.Kind)
	}
	return rule.To, ""
}

func (r *Reconciler) complete(ctx context.Context, vmsop *v1alpha2.VirtualMachineSnapshotOperation, resources []v1alpha2.SnapshotResourceStatus) (ctrl.Result, error) {
	vmsop.Status.Phase = v1alpha2.VMSOPPhaseCompleted
	vmsop.Status.Resources = resources
	meta.SetStatusCondition(&vmsop.Status.Conditions, metav1.Condition{
		Type:    string(vmsopcondition.TypeCompleted),
		Status:  metav1.ConditionTrue,
		Reason:  string(vmsopcondition.ReasonOperationCompleted),
		Message: "VirtualMachineSnapshotOperation completed.",
	})
	return ctrl.Result{}, r.patchStatus(ctx, vmsop)
}

func (r *Reconciler) fail(ctx context.Context, vmsop *v1alpha2.VirtualMachineSnapshotOperation, message string) (ctrl.Result, error) {
	vmsop.Status.Phase = v1alpha2.VMSOPPhaseFailed
	meta.SetStatusCondition(&vmsop.Status.Conditions, metav1.Condition{
		Type:    string(vmsopcondition.TypeCompleted),
		Status:  metav1.ConditionFalse,
		Reason:  string(vmsopcondition.ReasonOperationFailed),
		Message: message,
	})
	return ctrl.Result{}, r.patchStatus(ctx, vmsop)
}

func (r *Reconciler) progress(ctx context.Context, vmsop *v1alpha2.VirtualMachineSnapshotOperation, reason vmsopcondition.ReasonCompleted, message string) (ctrl.Result, error) {
	vmsop.Status.Phase = v1alpha2.VMSOPPhaseInProgress
	meta.SetStatusCondition(&vmsop.Status.Conditions, metav1.Condition{
		Type:    string(vmsopcondition.TypeCompleted),
		Status:  metav1.ConditionFalse,
		Reason:  reason.String(),
		Message: message,
	})
	if err := r.patchStatus(ctx, vmsop); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{RequeueAfter: requeueAfter}, nil
}

// renameVirtualMachine sets metadata.name on the single VirtualMachine object in objs from oldName to
// newName, and retargets every captured VirtualMachineBlockDeviceAttachment that referenced it, so the
// hotplugged disks attach to the restored machine instead of the (possibly still existing) source one.
// Every other object (in particular VirtualDisk) keeps its captured name unchanged — see the package
// doc comment.
func renameVirtualMachine(objs []unstructured.Unstructured, oldName, newName string) error {
	found := false
	for i := range objs {
		if objs[i].GetAPIVersion() != v1alpha2.SchemeGroupVersion.String() {
			continue
		}

		switch objs[i].GetKind() {
		case v1alpha2.VirtualMachineKind:
			if objs[i].GetName() != oldName {
				continue
			}
			objs[i].SetName(newName)
			found = true
		case v1alpha2.VirtualMachineBlockDeviceAttachmentKind:
			vmName, _, err := unstructured.NestedString(objs[i].Object, "spec", "virtualMachineName")
			if err != nil {
				return fmt.Errorf("read spec.virtualMachineName of VirtualMachineBlockDeviceAttachment %q: %w", objs[i].GetName(), err)
			}
			if vmName != oldName {
				continue
			}
			if err := unstructured.SetNestedField(objs[i].Object, newName, "spec", "virtualMachineName"); err != nil {
				return fmt.Errorf("set spec.virtualMachineName of VirtualMachineBlockDeviceAttachment %q: %w", objs[i].GetName(), err)
			}
		}
	}
	if !found {
		return fmt.Errorf("compiled restore manifests do not contain a VirtualMachine named %q", oldName)
	}
	return nil
}

func markCreatedBy(obj *unstructured.Unstructured, vmsopUID string) {
	anns := obj.GetAnnotations()
	if anns == nil {
		anns = map[string]string{}
	}
	anns[annCreatedByVMSOP] = vmsopUID
	obj.SetAnnotations(anns)
}

func resourceStatus(obj unstructured.Unstructured, phase v1alpha2.SnapshotResourceStatusPhase, message string) v1alpha2.SnapshotResourceStatus {
	return v1alpha2.SnapshotResourceStatus{
		APIVersion: obj.GetAPIVersion(),
		Kind:       obj.GetKind(),
		Name:       obj.GetName(),
		Status:     phase,
		Message:    message,
	}
}
