package gc

import (
	"context"
	"fmt"
	"time"

	dlog "github.com/deckhouse/deckhouse/pkg/log"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apiruntime "k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	clock "k8s.io/utils/clock/testing"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/deckhouse/virtualization-controller/pkg/common/pwgen"
	"github.com/deckhouse/virtualization-controller/pkg/common/testutil"
)

var _ = FDescribe("CronSource", func() {
	const (
		// Every day at 00:00
		scheduleSpec = "0 * * * *"
	)

	var (
		log        *dlog.Logger
		ctx        context.Context
		fakeClient client.Client
		mgr        *fakeGCManager
		fakeClock  *clock.FakeClock
		//queue      workqueue.RateLimitingInterface
	)

	BeforeEach(func() {
		log = testutil.NewNoOpLogger()
		ctx = testutil.ToContext(context.Background(), log)

		scheme := apiruntime.NewScheme()
		for _, f := range []func(*apiruntime.Scheme) error{
			clientgoscheme.AddToScheme,
			AddToScheme,
		} {
			Expect(f(scheme)).To(Succeed())
		}

		fakeClient = fake.NewClientBuilder().WithScheme(scheme).Build()

		mgr = newFakeGCManager(fakeClient, time.Hour, 10)
		fakeClock = clock.NewFakeClock(time.Now())
		//queue = workqueue.NewRateLimitingQueue(workqueue.DefaultControllerRateLimiter())
	})

	newSource := func(scheduleSpec string) *CronSource {
		source, err := NewCronSource(scheduleSpec, fakeClient, mgr, log)
		source.clock = fakeClock
		Expect(err).NotTo(HaveOccurred())
		return source
	}

	It("should enqueue 100 objects in completed state and delete them", func() {
		source := newSource(scheduleSpec)

		spawnFakeObjects(10, 10, fakeObjectPhaseCompleted, fakeClient)
		spawnFakeObjects(10, 10, fakeObjectPhasePending, fakeClient)
		spawnFakeObjects(10, 10, fakeObjectPhaseRunning, fakeClient)

		var enqueued []ctrl.Request
		source.addObjects(ctx, func(obj interface{}) {
			req := obj.(ctrl.Request)
			fakeObj := &FakeObject{}
			Expect(fakeClient.Get(ctx, req.NamespacedName, fakeObj)).To(Succeed())
			Expect(fakeObj.Phase).To(Equal(fakeObjectPhaseCompleted))
			enqueued = append(enqueued, req)
		})
		Expect(len(enqueued)).To(Equal(100))

		reconciler := NewReconciler(fakeClient, source, mgr)
		for _, req := range enqueued {
			result, err := reconciler.Reconcile(ctx, req)
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

	//It("should return the oldest object", func() {
	//	// Every day at 00:00
	//	scheduleSpec := "0 * * * *"
	//	source := newSource(scheduleSpec)
	//
	//	for i := range 2 {
	//		namespace := fmt.Sprintf("test-namespace-%d", i)
	//		for j := range 5 {
	//			obj := NewFakeObject(fmt.Sprintf("fake-%d", j), namespace)
	//			obj.Phase = fakeObjectPhaseCompleted
	//
	//			Expect(obj.Phase).To(Equal(fakeObjectPhaseCompleted))
	//			Expect(fakeClient.Create(context.Background(), obj)).To(Succeed())
	//		}
	//	}
	//
	//	Expect(source.Start(context.Background(), nil, queue)).To(Succeed())
	//	Expect(queue.Len()).To(Equal(0))
	//
	//	fakeClock.Step(time.Hour * 25)
	//
	//	Eventually(queue.Len()).WithTimeout(10 * time.Second).Should(Equal(20))
	//})
})

func spawnFakeObjects(countNamespaces, countPerNamespace int, phase string, client client.Client) {
	for i := 0; i < countNamespaces; i++ {
		namespace := fmt.Sprintf("test-namespace-%s-%d", pwgen.AlphaNum(32), i)
		for j := 0; j < countPerNamespace; j++ {
			obj := NewFakeObject(fmt.Sprintf("fake-%d", j), namespace)
			obj.Phase = phase
			Expect(client.Create(context.Background(), obj)).To(Succeed())
		}
	}
}
