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

package checksdnrouteforger

import (
	"context"
	"errors"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/tidwall/gjson"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/deckhouse/pkg/log"
	"github.com/deckhouse/module-sdk/pkg"
	"github.com/deckhouse/module-sdk/testing/mock"
	"github.com/deckhouse/virtualization-controller/pkg/common/network"
	"github.com/deckhouse/virtualization-controller/pkg/common/network/sdnapi"
)

func Test_CheckSDNRouteForger(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Check SDN Route Forger Suite")
}

var _ = Describe("Check SDN route forger hook", func() {
	var (
		snapshots *mock.SnapshotsMock
		values    *mock.OutputPatchableValuesCollectorMock
		dc        *mock.DependencyContainerMock
		metrics   *mock.MetricsCollectorMock
		// -1 while the hook has not set the metric.
		metricValue float64
	)

	newInput := func() *pkg.HookInput {
		return &pkg.HookInput{
			Snapshots: snapshots,
			Values:    values,
			DC:        dc,
			Logger:    log.NewNop(),

			MetricsCollector: metrics,
		}
	}

	prepareStateValuesEmpty := func() {
		values.GetMock.When(routeForgerStateValuesPath).Then(gjson.Result{Type: gjson.Null})
	}

	prepareStateValuesRecorded := func() {
		values.GetMock.When(routeForgerStateValuesPath).Then(gjson.Result{
			Type: gjson.String,
			Str:  `{"detectedAt":"2020-01-01T00:00:00Z"}`,
		})
	}

	prepareSnapshotEmpty := func() {
		snapshots.GetMock.When(moduleStateSecretSnapshot).Then([]pkg.Snapshot{})
	}

	prepareSnapshotWithData := func(data map[string][]byte) {
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      moduleStateSecretName,
				Namespace: "d8-virtualization",
			},
			Data: data,
		}

		snapshots.GetMock.When(moduleStateSecretSnapshot).Then([]pkg.Snapshot{
			mock.NewSnapshotMock(GinkgoT()).UnmarshalToMock.Set(func(v any) error {
				target, ok := v.(*corev1.Secret)
				Expect(ok).To(BeTrue())
				*target = *secret
				return nil
			}),
		})
	}

	prepareNetwork := func(conditions []metav1.Condition) {
		clusterNetwork := &sdnapi.ClusterNetwork{
			ObjectMeta: metav1.ObjectMeta{Name: network.MainClusterNetworkName},
			Status:     sdnapi.ClusterNetworkStatus{Conditions: conditions},
		}

		dc.GetK8sClientMock.Return(&fakeKubernetesClient{get: func(_ context.Context, key ctrlclient.ObjectKey, obj ctrlclient.Object) error {
			Expect(key.Name).To(Equal(network.MainClusterNetworkName))
			target, ok := obj.(*sdnapi.ClusterNetwork)
			Expect(ok).To(BeTrue())
			*target = *clusterNetwork
			return nil
		}}, nil)
	}

	prepareNetworkAbsent := func() {
		dc.GetK8sClientMock.Return(&fakeKubernetesClient{get: func(_ context.Context, _ ctrlclient.ObjectKey, _ ctrlclient.Object) error {
			return apierrors.NewNotFound(
				schema.GroupResource{Group: network.ClusterNetworkGVK.Group, Resource: "clusternetworks"},
				network.MainClusterNetworkName,
			)
		}}, nil)
	}

	prepareCRDAbsent := func() {
		dc.GetK8sClientMock.Return(&fakeKubernetesClient{get: func(_ context.Context, _ ctrlclient.ObjectKey, _ ctrlclient.Object) error {
			return &meta.NoKindMatchError{GroupKind: network.ClusterNetworkGVK.GroupKind()}
		}}, nil)
	}

	prepareReadFailure := func() {
		dc.GetK8sClientMock.Return(&fakeKubernetesClient{get: func(_ context.Context, _ ctrlclient.ObjectKey, _ ctrlclient.Object) error {
			return errors.New("the API server is unavailable")
		}}, nil)
	}

	routeForgerSupportedConditions := []metav1.Condition{
		{Type: routeForgerSupportedCondition, Status: metav1.ConditionTrue, Reason: "Supported"},
	}

	BeforeEach(func() {
		snapshots = mock.NewSnapshotsMock(GinkgoT())
		values = mock.NewOutputPatchableValuesCollectorMock(GinkgoT())
		dc = mock.NewDependencyContainerMock(GinkgoT())
		metricValue = -1
		metrics = mock.NewMetricsCollectorMock(GinkgoT())
		metrics.SetMock.Set(func(name string, value float64, _ map[string]string, _ ...pkg.MetricCollectorOption) {
			Expect(name).To(Equal(routeForgerSupportedMetric))
			metricValue = value
		})
	})

	Context("the sdn module is not in the cluster", func() {
		It("keeps shipping vm-route-forge when the ClusterNetwork CRD is absent", func() {
			prepareSnapshotEmpty()
			prepareStateValuesEmpty()
			prepareCRDAbsent()

			Expect(Reconcile(context.Background(), newInput())).To(Succeed())
			Expect(metricValue).To(Equal(0.0))
			// No Set call is expected: the mock fails the test on an unexpected call.
		})

		It("keeps shipping vm-route-forge when the Main ClusterNetwork is absent", func() {
			prepareSnapshotEmpty()
			prepareStateValuesEmpty()
			prepareNetworkAbsent()

			Expect(Reconcile(context.Background(), newInput())).To(Succeed())
		})
	})

	Context("the Main ClusterNetwork exists", func() {
		It("keeps shipping vm-route-forge without the condition", func() {
			prepareSnapshotEmpty()
			prepareStateValuesEmpty()
			prepareNetwork(nil)

			Expect(Reconcile(context.Background(), newInput())).To(Succeed())
		})

		It("keeps shipping vm-route-forge while the condition is False", func() {
			prepareSnapshotEmpty()
			prepareStateValuesEmpty()
			prepareNetwork([]metav1.Condition{
				{Type: routeForgerSupportedCondition, Status: metav1.ConditionFalse, Reason: "AgentsNotReady"},
			})

			Expect(Reconcile(context.Background(), newInput())).To(Succeed())
		})

		It("ignores conditions of other types", func() {
			prepareSnapshotEmpty()
			prepareStateValuesEmpty()
			prepareNetwork([]metav1.Condition{
				{Type: "Ready", Status: metav1.ConditionTrue},
			})

			Expect(Reconcile(context.Background(), newInput())).To(Succeed())
		})

		It("records the state once the condition is True", func() {
			prepareSnapshotEmpty()
			prepareStateValuesEmpty()
			prepareNetwork(routeForgerSupportedConditions)

			var stateSetInValues bool
			values.SetMock.Set(func(path string, _ any) {
				Expect(path).To(Equal(routeForgerStateValuesPath))
				stateSetInValues = true
			})

			Expect(Reconcile(context.Background(), newInput())).To(Succeed())
			Expect(stateSetInValues).To(BeTrue(), "the hook must record the state so the DaemonSet stops being rendered")
			Expect(metricValue).To(Equal(1.0), "the alert on the missing DaemonSet must go silent")
		})
	})

	Context("the state is already recorded", func() {
		It("keeps the state from the Secret and does not read the cluster", func() {
			prepareSnapshotWithData(map[string][]byte{
				routeForgerStateSecretKey: []byte(`{"detectedAt":"2020-01-01T00:00:00Z"}`),
			})

			var stateSetInValues bool
			values.SetMock.Set(func(path string, _ any) {
				Expect(path).To(Equal(routeForgerStateValuesPath))
				stateSetInValues = true
			})

			// No k8s client mock is prepared on purpose: reading the cluster here
			// would fail the test, and the recorded state must be enough.
			Expect(Reconcile(context.Background(), newInput())).To(Succeed())
			Expect(stateSetInValues).To(BeTrue())
			Expect(metricValue).To(Equal(1.0))
		})

		It("does not confuse the state of another hook with its own", func() {
			prepareSnapshotWithData(map[string][]byte{
				"vmClassGenericInstallation": []byte(`{"installedAt":"2020-01-01T00:00:00Z"}`),
			})
			prepareStateValuesEmpty()
			prepareNetworkAbsent()

			Expect(Reconcile(context.Background(), newInput())).To(Succeed())
		})

		It("leaves the values alone when the Secret is gone but the state is in values", func() {
			prepareSnapshotEmpty()
			prepareStateValuesRecorded()

			// Neither a cluster read nor a Set is expected: helm re-creates the
			// Secret from the values that are already there.
			Expect(Reconcile(context.Background(), newInput())).To(Succeed())
			Expect(metricValue).To(Equal(1.0))
		})
	})

	Context("the cluster cannot be read", func() {
		It("keeps shipping vm-route-forge and retries later", func() {
			prepareSnapshotEmpty()
			prepareStateValuesEmpty()
			prepareReadFailure()

			// A read error is not an evidence that sdn took the routing over:
			// dropping the DaemonSet here would leave the fleet without routes.
			Expect(Reconcile(context.Background(), newInput())).To(Succeed())
		})
	})
})

type fakeKubernetesClient struct {
	pkg.KubernetesClient
	get func(ctx context.Context, key ctrlclient.ObjectKey, obj ctrlclient.Object) error
}

func (f *fakeKubernetesClient) Get(ctx context.Context, key ctrlclient.ObjectKey, obj ctrlclient.Object, _ ...ctrlclient.GetOption) error {
	return f.get(ctx, key, obj)
}
