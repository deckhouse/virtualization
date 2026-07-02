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

package vmpool

import (
	"context"
	"encoding/json"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	admissionv1 "k8s.io/api/admission/v1"
	autoscalingv1 "k8s.io/api/autoscaling/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"

	"github.com/deckhouse/virtualization-controller/pkg/common/testutil"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

func scaleRaw(replicas int32) runtime.RawExtension {
	raw, err := json.Marshal(&autoscalingv1.Scale{Spec: autoscalingv1.ScaleSpec{Replicas: replicas}})
	Expect(err).NotTo(HaveOccurred())
	return runtime.RawExtension{Raw: raw}
}

func scaleUpdateRequest(oldReplicas, newReplicas int32) admission.Request {
	return admission.Request{AdmissionRequest: admissionv1.AdmissionRequest{
		Operation:   admissionv1.Update,
		SubResource: "scale",
		Namespace:   "ci",
		Name:        "web",
		Object:      scaleRaw(newReplicas),
		OldObject:   scaleRaw(oldReplicas),
	}}
}

func poolWithPolicy(policy v1alpha2.ScaleDownPolicy) *v1alpha2.VirtualMachinePool {
	return &v1alpha2.VirtualMachinePool{
		ObjectMeta: metav1.ObjectMeta{Name: "web", Namespace: "ci"},
		Spec:       v1alpha2.VirtualMachinePoolSpec{ScaleDownPolicy: policy},
	}
}

var _ = Describe("scaleValidator", func() {
	var ctx context.Context

	BeforeEach(func() { ctx = context.Background() })

	validatorFor := func(pool *v1alpha2.VirtualMachinePool) *scaleValidator {
		c, err := testutil.NewFakeClientWithObjects(pool)
		Expect(err).NotTo(HaveOccurred())
		return &scaleValidator{client: c}
	}

	It("denies a decrease for an Explicit pool", func() {
		resp := validatorFor(poolWithPolicy(v1alpha2.ScaleDownPolicyExplicit)).Handle(ctx, scaleUpdateRequest(5, 3))
		Expect(resp.Allowed).To(BeFalse())
		Expect(string(resp.Result.Message)).To(ContainSubstring("scaleDownWith"))
	})

	It("allows a decrease for a NewestFirst pool", func() {
		resp := validatorFor(poolWithPolicy(v1alpha2.ScaleDownPolicyNewestFirst)).Handle(ctx, scaleUpdateRequest(5, 3))
		Expect(resp.Allowed).To(BeTrue())
	})

	It("allows an increase even for an Explicit pool", func() {
		resp := validatorFor(poolWithPolicy(v1alpha2.ScaleDownPolicyExplicit)).Handle(ctx, scaleUpdateRequest(3, 5))
		Expect(resp.Allowed).To(BeTrue())
	})

	It("allows a no-op (equal replicas)", func() {
		resp := validatorFor(poolWithPolicy(v1alpha2.ScaleDownPolicyExplicit)).Handle(ctx, scaleUpdateRequest(3, 3))
		Expect(resp.Allowed).To(BeTrue())
	})

	It("ignores non-scale subresource requests", func() {
		req := scaleUpdateRequest(5, 3)
		req.SubResource = ""
		resp := validatorFor(poolWithPolicy(v1alpha2.ScaleDownPolicyExplicit)).Handle(ctx, req)
		Expect(resp.Allowed).To(BeTrue())
	})
})
