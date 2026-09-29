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

package handler

import (
	"context"
	"errors"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/validation/field"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	"github.com/deckhouse/virtualization-controller/pkg/common/testutil"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vmpool/internal/expectations"
	"github.com/deckhouse/virtualization-controller/pkg/controller/vmpool/internal/poollabels"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmpoolcondition"
)

var vmGroupKind = schema.GroupKind{Group: v1alpha2.SchemeGroupVersion.Group, Kind: v1alpha2.VirtualMachineKind}

// policyDenial builds the error kube-apiserver returns when a ValidatingAdmissionPolicy
// denies a request: the reason the policy is configured with and the message as a cause
// without a field.
func policyDenial(reason metav1.StatusReason) error {
	msg := "ValidatingAdmissionPolicy 'even-cores' with binding 'even-cores' denied request: the number of cores must be even"
	err := apierrors.NewForbidden(schema.GroupResource{}, "x", errors.New(msg))
	err.ErrStatus.Reason = reason
	err.ErrStatus.Details.Causes = append(err.ErrStatus.Details.Causes, metav1.StatusCause{Message: msg})
	return err
}

var _ = Describe("Report", func() {
	DescribeTable("tells persistent errors from transient ones",
		func(err error, persistent bool) {
			Expect(isPersistent(err)).To(Equal(persistent))
		},
		Entry("schema rejection", apierrors.NewInvalid(vmGroupKind, "x", nil), true),
		Entry("quota", apierrors.NewForbidden(schema.GroupResource{}, "x", errors.New("exceeded quota: q")), true),
		Entry("bad request", apierrors.NewBadRequest("bad"), true),
		Entry("conflict", apierrors.NewConflict(schema.GroupResource{}, "x", errors.New("changed")), false),
		Entry("webhook unavailable", apierrors.NewInternalError(errors.New(`failed calling webhook "vm"`)), false),
		Entry("timeout", apierrors.NewTimeoutError("slow", 1), false),
		Entry("not an API error", errors.New("boom"), false),
	)

	DescribeTable("explains an error without the raw API response",
		func(err error, want string) {
			Expect(explain(err, vmTemplatePath)).To(Equal(want))
		},
		Entry("schema rejection names the field at the end",
			apierrors.NewInvalid(vmGroupKind, "x", field.ErrorList{field.NotSupported(field.NewPath("spec", "liveMigrationPolicy"), "Manual", []string{"PreferSafe"})}),
			`the template is rejected: Unsupported value: "Manual": supported values: "PreferSafe" (spec.virtualMachineTemplate.spec.liveMigrationPolicy). Fix the template.`),
		Entry("quota does not claim which quota",
			apierrors.NewForbidden(schema.GroupResource{Resource: "virtualdisks"}, "x", errors.New("exceeded quota: storage, requested: count/virtualdisks=1")),
			"the namespace ResourceQuota is exceeded. The pool retries automatically; contact your administrator to increase the quota."),
		Entry("webhook denial keeps only the webhook's own words",
			apierrors.NewForbidden(schema.GroupResource{}, "x", errors.New(`admission webhook "vm.virtualization-controller.validate.d8-virtualization" denied the request: the value 3% is not allowed.`)),
			"the template is rejected: the value 3% is not allowed. Fix the template."),
		Entry("other rejection keeps its message and asks to fix the template",
			apierrors.NewBadRequest("spec.cpu is malformed"),
			"the request is rejected: spec.cpu is malformed. Fix the template."),
		Entry("schema rejection without field details keeps the message",
			apierrors.NewInvalid(vmGroupKind, "x", nil),
			`the request is rejected: VirtualMachine.virtualization.deckhouse.io "x" is invalid. Fix the template.`),
		Entry("other access denial goes to the administrator",
			apierrors.NewForbidden(schema.GroupResource{}, "x", errors.New("user cannot create")),
			"access is denied. Contact your administrator."),
		Entry("policy denial with the default reason keeps only the policy's own words",
			policyDenial(metav1.StatusReasonInvalid),
			"the template is rejected: the number of cores must be even. Fix the template."),
		Entry("policy denial with the Forbidden reason is not taken for missing access",
			policyDenial(metav1.StatusReasonForbidden),
			"the template is rejected: the number of cores must be even. Fix the template."),
	)

	DescribeTable("explains a failed deletion without blaming the template",
		func(err error, want string) {
			Expect(explainDeletion(err)).To(Equal(want))
		},
		Entry("webhook denial",
			apierrors.NewForbidden(schema.GroupResource{}, "x", errors.New(`admission webhook "vm" denied the request: the machine is protected.`)),
			"the deletion is denied: the machine is protected."),
		Entry("access denial", apierrors.NewForbidden(schema.GroupResource{}, "x", errors.New("user cannot delete")), "access is denied. Contact your administrator."),
		Entry("other rejection", apierrors.NewBadRequest("bad"), "the request is rejected: bad."),
		Entry("policy denial with the default reason", policyDenial(metav1.StatusReasonInvalid), "the deletion is denied: the number of cores must be even."),
		Entry("policy denial with the Forbidden reason", policyDenial(metav1.StatusReasonForbidden), "the deletion is denied: the number of cores must be even."),
	)

	It("keeps the first failure of a kind", func() {
		r := &passReport{}
		r.CreationFailed("first")
		r.CreationFailed("second")
		Expect(r.creation).To(Equal("first"))
	})
})

var _ = Describe("Failures reach the pool status", func() {
	var ctx context.Context
	BeforeEach(func() { ctx = WithReport(context.Background()) })

	runStatus := func(c client.Client, pool *v1alpha2.VirtualMachinePool) {
		_, err := NewStatusHandler(c).Handle(ctx, pool)
		Expect(err).NotTo(HaveOccurred())
	}

	It("reports a rejected replica creation in Progressing", func() {
		pool := newPool(1)
		c, err := testutil.NewFakeClientWithInterceptorWithObjects(interceptor.Funcs{
			Create: func(context.Context, client.WithWatch, client.Object, ...client.CreateOption) error {
				return apierrors.NewForbidden(schema.GroupResource{}, "x", errors.New("exceeded quota: q"))
			},
		}, pool)
		Expect(err).NotTo(HaveOccurred())

		_, err = NewSyncHandler(c, expectations.New(), testRecorder()).Handle(ctx, pool)
		Expect(err).To(HaveOccurred())
		runStatus(c, pool)

		Expect(conditionReason(pool, vmpoolcondition.TypeProgressing)).To(Equal(vmpoolcondition.ReasonReplicaCreationFailed.String()))
		Expect(conditionMessage(pool, vmpoolcondition.TypeProgressing)).To(Equal(
			"Scaling up: 0 of 1 replicas are created. Cannot create a VirtualMachine from the template: the namespace ResourceQuota is exceeded. The pool retries automatically; contact your administrator to increase the quota."))
	})

	It("reports a replica that cannot be deleted", func() {
		pool := newPool(1)
		c, err := testutil.NewFakeClientWithInterceptorWithObjects(interceptor.Funcs{
			Delete: func(context.Context, client.WithWatch, client.Object, ...client.DeleteOption) error {
				return apierrors.NewForbidden(schema.GroupResource{}, "x", errors.New("user cannot delete"))
			},
		}, pool,
			newMemberVM(pool, "web-a", v1alpha2.MachineRunning, referenceTime, false),
			newMemberVM(pool, "web-b", v1alpha2.MachineRunning, referenceTime.Add(time.Minute), false),
		)
		Expect(err).NotTo(HaveOccurred())

		_, err = NewSyncHandler(c, expectations.New(), testRecorder()).Handle(ctx, pool)
		Expect(err).To(HaveOccurred())
		runStatus(c, pool)

		Expect(conditionReason(pool, vmpoolcondition.TypeProgressing)).To(Equal(vmpoolcondition.ReasonReplicaDeletionFailed.String()))
		Expect(conditionMessage(pool, vmpoolcondition.TypeProgressing)).To(HaveSuffix(`Cannot delete VirtualMachine "web-b": access is denied. Contact your administrator.`))
	})

	It("reports a template the replica rejects", func() {
		pool := newPool(1)
		vm := newMemberVM(pool, "web-a", v1alpha2.MachineRunning, referenceTime, false)
		c, err := testutil.NewFakeClientWithInterceptorWithObjects(interceptor.Funcs{
			Update: func(context.Context, client.WithWatch, client.Object, ...client.UpdateOption) error {
				return apierrors.NewInvalid(vmGroupKind, "web-a", field.ErrorList{field.Invalid(field.NewPath("spec", "memory", "size"), "1Gi", "below the class minimum")})
			},
		}, pool, vm)
		Expect(err).NotTo(HaveOccurred())

		_, err = NewTemplateHandler(c).Handle(ctx, pool)
		Expect(err).To(HaveOccurred())
		runStatus(c, pool)

		Expect(conditionReason(pool, vmpoolcondition.TypeSynced)).To(Equal(vmpoolcondition.ReasonReplicaUpdateFailed.String()))
		Expect(conditionMessage(pool, vmpoolcondition.TypeSynced)).To(ContainSubstring(`Cannot apply the template to VirtualMachine "web-a": the template is rejected:`))
	})

	It("files a Retain disk failure of a new replica under Progressing", func() {
		pool := newPool(1)
		pool.Spec.VirtualDiskTemplates = []v1alpha2.VirtualDiskTemplateSpec{retainTemplate("cache", 0, nil)}
		vm := withDiskRefs(newMemberVM(pool, "web-a", v1alpha2.MachinePending, referenceTime, false), "cache")
		c := clientRejectingDisks(pool, vm)

		_, _ = NewDisksHandler(c).Handle(ctx, pool)
		runStatus(c, pool)

		Expect(conditionReason(pool, vmpoolcondition.TypeProgressing)).To(Equal(vmpoolcondition.ReasonReplicaCreationFailed.String()))
		Expect(conditionMessage(pool, vmpoolcondition.TypeProgressing)).To(MatchRegexp(`Cannot create VirtualDisk "web-cache-\w+": the namespace ResourceQuota is exceeded`))
	})

	It("reports a disk that cannot be attached to a new replica", func() {
		pool := newPool(1)
		pool.Spec.VirtualDiskTemplates = []v1alpha2.VirtualDiskTemplateSpec{diskTemplate("root", v1alpha2.VirtualDiskReclaimDelete)}
		vm := withDiskRefs(newMemberVM(pool, "web-a", v1alpha2.MachinePending, referenceTime, false), "root")
		c, err := testutil.NewFakeClientWithInterceptorWithObjects(interceptor.Funcs{
			Update: func(ctx context.Context, cl client.WithWatch, obj client.Object, opts ...client.UpdateOption) error {
				if _, ok := obj.(*v1alpha2.VirtualMachine); ok {
					return apierrors.NewForbidden(schema.GroupResource{}, obj.GetName(), errors.New(`admission webhook "vm" denied the request: too many disks`))
				}
				return cl.Update(ctx, obj, opts...)
			},
		}, pool, vm)
		Expect(err).NotTo(HaveOccurred())

		_, _ = NewDisksHandler(c).Handle(ctx, pool)
		runStatus(c, pool)

		Expect(conditionMessage(pool, vmpoolcondition.TypeProgressing)).To(HaveSuffix(`Cannot attach VirtualDisk "web-a-root" to VirtualMachine "web-a": the template is rejected: too many disks. Fix the template.`))
	})

	It("points a rejected disk to its path in the pool spec", func() {
		pool := newPool(1)
		pool.Spec.VirtualDiskTemplates = []v1alpha2.VirtualDiskTemplateSpec{diskTemplate("root", v1alpha2.VirtualDiskReclaimDelete)}
		vm := withDiskRefs(newMemberVM(pool, "web-a", v1alpha2.MachinePending, referenceTime, false), "root")
		c, err := testutil.NewFakeClientWithInterceptorWithObjects(interceptor.Funcs{
			Create: func(context.Context, client.WithWatch, client.Object, ...client.CreateOption) error {
				return apierrors.NewInvalid(vmGroupKind, "web-a-root", field.ErrorList{field.Invalid(field.NewPath("spec", "persistentVolumeClaim", "size"), "1Ki", "too small")})
			},
		}, pool, vm)
		Expect(err).NotTo(HaveOccurred())

		_, _ = NewDisksHandler(c).Handle(ctx, pool)
		runStatus(c, pool)

		Expect(conditionMessage(pool, vmpoolcondition.TypeProgressing)).To(ContainSubstring("(spec.virtualDiskTemplates[root].spec.persistentVolumeClaim.size)"))
	})

	It("does not report a transient failure to create a disk", func() {
		pool := newPool(1)
		pool.Spec.VirtualDiskTemplates = []v1alpha2.VirtualDiskTemplateSpec{diskTemplate("root", v1alpha2.VirtualDiskReclaimDelete)}
		vm := withDiskRefs(newMemberVM(pool, "web-a", v1alpha2.MachinePending, referenceTime, false), "root")
		c, err := testutil.NewFakeClientWithInterceptorWithObjects(interceptor.Funcs{
			Create: func(context.Context, client.WithWatch, client.Object, ...client.CreateOption) error {
				return apierrors.NewInternalError(errors.New(`failed calling webhook "vd"`))
			},
		}, pool, vm)
		Expect(err).NotTo(HaveOccurred())

		_, _ = NewDisksHandler(c).Handle(ctx, pool)
		runStatus(c, pool)

		Expect(conditionReason(pool, vmpoolcondition.TypeProgressing)).To(Equal(vmpoolcondition.ReasonReplicasProgressing.String()))
	})

	It("does not report a conflict while applying the template", func() {
		pool := newPool(1)
		vm := newMemberVM(pool, "web-a", v1alpha2.MachineRunning, referenceTime, false)
		c, err := testutil.NewFakeClientWithInterceptorWithObjects(interceptor.Funcs{
			Update: func(context.Context, client.WithWatch, client.Object, ...client.UpdateOption) error {
				return apierrors.NewConflict(schema.GroupResource{}, "web-a", errors.New("changed"))
			},
		}, pool, vm)
		Expect(err).NotTo(HaveOccurred())

		_, err = NewTemplateHandler(c).Handle(ctx, pool)
		Expect(err).To(HaveOccurred())
		runStatus(c, pool)

		Expect(conditionReason(pool, vmpoolcondition.TypeSynced)).To(Equal(vmpoolcondition.ReasonRolloutInProgress.String()))
	})

	It("files a disk failure of a new replica under Progressing", func() {
		pool := newPool(1)
		pool.Spec.VirtualDiskTemplates = []v1alpha2.VirtualDiskTemplateSpec{diskTemplate("root", v1alpha2.VirtualDiskReclaimDelete)}
		vm := withDiskRefs(newMemberVM(pool, "web-a", v1alpha2.MachinePending, referenceTime, false), "root")
		c := clientRejectingDisks(pool, vm)

		_, _ = NewDisksHandler(c).Handle(ctx, pool)
		runStatus(c, pool)

		Expect(conditionReason(pool, vmpoolcondition.TypeProgressing)).To(Equal(vmpoolcondition.ReasonReplicaCreationFailed.String()))
		Expect(conditionMessage(pool, vmpoolcondition.TypeProgressing)).To(ContainSubstring(`Cannot create VirtualDisk "web-a-root": the namespace ResourceQuota is exceeded.`))
	})

	It("files a disk failure of an existing replica under Synced", func() {
		pool := newPool(1)
		pool.Spec.VirtualDiskTemplates = []v1alpha2.VirtualDiskTemplateSpec{
			diskTemplate("root", v1alpha2.VirtualDiskReclaimDelete),
			diskTemplate("data", v1alpha2.VirtualDiskReclaimDelete),
		}
		vm := withDiskRefs(newMemberVM(pool, "web-a", v1alpha2.MachineRunning, referenceTime, false), "web-a-root")
		vm.Labels[poollabels.TemplateHash] = poollabels.ComputeTemplateHash(pool)
		c := clientRejectingDisks(pool, vm, labeledDisk(pool, "web-a-root", "root"))

		_, _ = NewDisksHandler(c).Handle(ctx, pool)
		runStatus(c, pool)

		Expect(meta.FindStatusCondition(pool.Status.Conditions, vmpoolcondition.TypeProgressing.String())).To(BeNil())
		Expect(conditionReason(pool, vmpoolcondition.TypeSynced)).To(Equal(vmpoolcondition.ReasonReplicaUpdateFailed.String()))
		Expect(conditionMessage(pool, vmpoolcondition.TypeSynced)).To(ContainSubstring(`Cannot create VirtualDisk "web-a-data"`))
	})
})

func clientRejectingDisks(objs ...client.Object) client.Client {
	c, err := testutil.NewFakeClientWithInterceptorWithObjects(interceptor.Funcs{
		Create: func(ctx context.Context, cl client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
			if _, ok := obj.(*v1alpha2.VirtualDisk); ok {
				return apierrors.NewForbidden(schema.GroupResource{Resource: "virtualdisks"}, obj.GetName(), errors.New("exceeded quota: q"))
			}
			return cl.Create(ctx, obj, opts...)
		},
	}, objs...)
	Expect(err).NotTo(HaveOccurred())
	return c
}
