package gc

import (
	"context"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/types"
	clock "k8s.io/utils/clock/testing"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	dlog "github.com/deckhouse/deckhouse/pkg/log"
	"github.com/deckhouse/virtualization-controller/pkg/common/testutil"
)

var _ = Describe("GCReconciler", func() {
	var (
		log        *dlog.Logger
		ctx        context.Context
		fakeClient client.Client
		mgr        *fakeGCManager
		fakeClock  *clock.FakeClock
	)

	BeforeEach(func() {
		log = testutil.NewNoOpLogger()
		ctx = testutil.ToContext(context.Background(), log)

		scheme := newScheme()
		fakeClient = fake.NewClientBuilder().WithScheme(scheme).Build()

		mgr = newFakeGCManager(fakeClient, 24*time.Hour, 10)

		fakeClock = clock.NewFakeClock(time.Now())
	})

	It("should enqueue 100 objects in completed state and delete them", func() {
		spawnFakeObjects(10, 10, fakeObjectPhaseCompleted, fakeClient, fakeClock)
		spawnFakeObjects(10, 10, fakeObjectPhasePending, fakeClient, fakeClock)
		spawnFakeObjects(10, 10, fakeObjectPhaseRunning, fakeClient, fakeClock)

		beforeReconcileObjects := &FakeObjectList{}
		Expect(fakeClient.List(ctx, beforeReconcileObjects)).To(Succeed())
		Expect(beforeReconcileObjects.Items).To(HaveLen(300))

		reconciler := NewReconciler(fakeClient, nil, mgr)
		for _, obj := range beforeReconcileObjects.Items {
			result, err := reconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Namespace: obj.GetNamespace(),
					Name:      obj.GetName(),
				},
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.IsZero()).To(BeTrue())
		}

		objs := &FakeObjectList{}
		Expect(fakeClient.List(ctx, objs)).To(Succeed())
		Expect(len(objs.Items)).To(Equal(200))
		for _, obj := range objs.Items {
			Expect(obj.Phase).NotTo(Equal(fakeObjectPhaseCompleted))
		}
	})
})
