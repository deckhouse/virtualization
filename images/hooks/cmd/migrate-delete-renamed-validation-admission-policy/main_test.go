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

package main

import (
	"context"
	"testing"

	"github.com/deckhouse/deckhouse/pkg/log"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/module-sdk/pkg"
	"github.com/deckhouse/module-sdk/testing/mock"
)

func TestMigrateDeleteRenamedValidationAdmissionPolicy(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "MigrateDeleteRenamedValidationAdmissionPolicy Suite")
}

var _ = FDescribe("Test ValidatingAdmissionPolicy/ValidatingAdmissionPolicyBinding", func() {
	var (
		dc        *mock.DependencyContainerMock
		snapshots *mock.SnapshotsMock
	)

	setSnapshots := func(snapPolicy, snapBinding pkg.Snapshot) {
		snapshots.GetMock.When(POLICY_SNAPSHOT_NAME).Then([]pkg.Snapshot{snapPolicy})
		snapshots.GetMock.When(BINDING_SNAPSHOT_NAME).Then([]pkg.Snapshot{snapBinding})
	}

	newSnapshotPolicy := func(labels map[string]string) pkg.Snapshot {
		snap := mock.NewSnapshotMock(GinkgoT())
		snap.UnmarshalToMock.Set(func(v any) (err error) {
			data, ok := v.(*unstructured.Unstructured)
			Expect(ok).To(BeTrue())
			data.SetName(POLICY_SNAPSHOT_NAME)
			data.SetKind("ValidatingAdmissionPolicy")
			data.SetAPIVersion("admissionregistration.k8s.io/v1")
			data.SetLabels(labels)
			return nil
		})
		return snap
	}

	newSnapshotBinding := func(labels map[string]string) pkg.Snapshot {
		snap := mock.NewSnapshotMock(GinkgoT())
		snap.UnmarshalToMock.Set(func(v any) (err error) {
			data, ok := v.(*unstructured.Unstructured)
			Expect(ok).To(BeTrue())
			data.SetName(BINDING_SNAPSHOT_NAME)
			data.SetKind("ValidatingAdmissionPolicyBinding")
			data.SetAPIVersion("admissionregistration.k8s.io/v1")
			data.SetLabels(labels)
			return nil
		})
		return snap
	}

	newInput := func() *pkg.HookInput {
		return &pkg.HookInput{
			Snapshots: snapshots,
			DC:        dc,
			Logger:    log.NewNop(),
		}
	}

	BeforeEach(func() {
		dc = mock.NewDependencyContainerMock(GinkgoT())
		snapshots = mock.NewSnapshotsMock(GinkgoT())
	})

	AfterEach(func() {
		dc = nil
		snapshots = nil
	})

	DescribeTable("test with labels", func(policyLabels map[string]string, policyShouldDelete bool, bindingLabels map[string]string, bindingShouldDelete bool) {
		setSnapshots(newSnapshotPolicy(policyLabels), newSnapshotBinding(bindingLabels))
		if policyShouldDelete || bindingShouldDelete {
			dc.GetK8sClientMock.Set(func(options ...pkg.KubernetesOption) (pkg.KubernetesClient, error) {
				return mock.NewKubernetesClientMock(GinkgoT()).DeleteMock.Set(func(ctx context.Context, obj client.Object, opts ...client.DeleteOption) (err error) {
					labelExist := obj.GetLabels()[managed_by_label] == managed_by_label_value

					switch obj.GetObjectKind().GroupVersionKind().Kind {
					case "ValidatingAdmissionPolicy":
						Expect(labelExist).To(Equal(policyShouldDelete))
					case "ValidatingAdmissionPolicyBinding":
						Expect(labelExist).To(Equal(bindingShouldDelete))
					default:
						Fail("unexpected kind")
					}
					return nil
				}), nil
			})
		} else {
			dc.GetK8sClientMock.Set(func(options ...pkg.KubernetesOption) (pkg.KubernetesClient, error) {
				return mock.NewKubernetesClientMock(GinkgoT()), nil
			})
		}
		Expect(reconcile(context.Background(), newInput())).To(Succeed())

	},
		Entry("not should delete",
			map[string]string{"test": "test"},
			false,
			map[string]string{"test": "test"},
			false,
		),
		Entry("should delete policy",
			map[string]string{managed_by_label: managed_by_label_value},
			true,
			map[string]string{"test": "test"},
			false,
		),
		Entry("should delete binding",
			map[string]string{"test": "test"},
			false,
			map[string]string{managed_by_label: managed_by_label_value},
			true,
		),
		Entry("should delete all",
			map[string]string{managed_by_label: managed_by_label_value},
			true,
			map[string]string{managed_by_label: managed_by_label_value},
			true,
		),
	)

	//It("test with labels test:test", func() {
	//	setSnapshots(
	//		[]pkg.Snapshot{newSnapshotPolicy(map[string]string{"test": "test"})},
	//		[]pkg.Snapshot{newSnapshotBinding(map[string]string{"test": "test"})})
	//	Expect(reconcile(context.Background(), newInput())).To(Succeed())
	//})
	//
	//It("test with labels app.kubernetes.io/managed-by:\"\"", func() {
	//	setSnapshots(
	//		[]pkg.Snapshot{newSnapshotPolicy(map[string]string{managed_by_label: managed_by_label_value})},
	//		[]pkg.Snapshot{newSnapshotBinding(map[string]string{managed_by_label: managed_by_label_value})})
	//	Expect(reconcile(context.Background(), newInput())).To(Succeed())
	//})
	//
	//It("should failed when get k8s client", func() {
	//	clientErr := errors.New("client error")
	//	dc.GetK8sClientMock.Set(func(options ...pkg.KubernetesOption) (pkg.KubernetesClient, error) {
	//		return nil, clientErr
	//	})
	//	setSnapshots(
	//		[]pkg.Snapshot{newSnapshotPolicy(map[string]string{"test": "test"})},
	//		[]pkg.Snapshot{newSnapshotBinding(map[string]string{"test": "test"})})
	//	err := reconcile(context.Background(), newInput())
	//	Expect(err).To(MatchError(clientErr))
	//})
	//
	//It(fmt.Sprintf("sohuld skip vap %s", POLICY_SNAPSHOT_NAME), func() {
	//	setSnapshots(
	//		[]pkg.Snapshot{newSnapshotPolicy(map[string]string{"test": "test"})},
	//		[]pkg.Snapshot{newSnapshotBinding(map[string]string{"test": "test"})})
	//	Expect(reconcile(context.Background(), newInput())).To(Succeed())
	//
	//	dc.GetK8sClientMock.Set(func(options ...pkg.KubernetesOption) (pkg.KubernetesClient, error) {
	//		c := mock.NewKubernetesClientMock(GinkgoT())
	//		c.DeleteMock.Set(
	//			func(ctx context.Context, obj client.Object, opts ...client.DeleteOption) (err error) {
	//				vap, ok := obj.(*admissionregistrationv1.ValidatingAdmissionPolicy)
	//				Expect(ok).To(BeTrue())
	//				Expect(vap.Name).To(Equal(POLICY_SNAPSHOT_NAME))
	//				return nil
	//			})
	//		return c, nil
	//	})
	//})
	//It(fmt.Sprintf("sohuld skip vap %s", BINDING_SNAPSHOT_NAME), func() {
	//	setSnapshots(
	//		[]pkg.Snapshot{newSnapshotPolicy(map[string]string{"test": "test"})},
	//		[]pkg.Snapshot{newSnapshotBinding(map[string]string{"test": "test"})})
	//	Expect(reconcile(context.Background(), newInput())).To(Succeed())
	//
	//	dc.GetK8sClientMock.Set(func(options ...pkg.KubernetesOption) (pkg.KubernetesClient, error) {
	//		c := mock.NewKubernetesClientMock(GinkgoT())
	//		c.DeleteMock.Set(
	//			func(ctx context.Context, obj client.Object, opts ...client.DeleteOption) (err error) {
	//				vap, ok := obj.(*admissionregistrationv1.ValidatingAdmissionPolicyBinding)
	//				Expect(ok).To(BeTrue())
	//				Expect(vap.Name).To(Equal(BINDING_SNAPSHOT_NAME))
	//				return nil
	//			})
	//		return c, nil
	//	})
	//})
	//It(fmt.Sprintf("sohuld delete vap %s with labels", BINDING_SNAPSHOT_NAME), func() {
	//	setSnapshots(
	//		[]pkg.Snapshot{newSnapshotPolicy(map[string]string{managed_by_label: managed_by_label_value})},
	//		[]pkg.Snapshot{newSnapshotBinding(map[string]string{managed_by_label: managed_by_label_value})})
	//	Expect(reconcile(context.Background(), newInput())).To(Succeed())
	//
	//	dc.GetK8sClientMock.Set(func(options ...pkg.KubernetesOption) (pkg.KubernetesClient, error) {
	//		c := mock.NewKubernetesClientMock(GinkgoT())
	//		c.DeleteMock.Set(
	//			func(ctx context.Context, obj client.Object, opts ...client.DeleteOption) (err error) {
	//				vap, ok := obj.(*admissionregistrationv1.ValidatingAdmissionPolicyBinding)
	//				Expect(ok).To(BeTrue())
	//				Expect(vap.Name).To(Equal(BINDING_SNAPSHOT_NAME))
	//				return nil
	//			})
	//		return c, nil
	//	})
	//})
})
