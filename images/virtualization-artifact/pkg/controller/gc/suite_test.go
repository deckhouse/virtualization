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

package gc

import (
	"context"
	"fmt"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apiruntime "k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/util/workqueue"
	clock "k8s.io/utils/clock/testing"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/common/pwgen"
)

func TestGC(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "GC Suite")
}

func spawnFakeObjects(countNamespaces, countPerNamespace int, phase string, client client.Client, fakeClock *clock.FakeClock) {
	GinkgoHelper()

	for i := 0; i < countNamespaces; i++ {
		namespace := fmt.Sprintf("test-namespace-%s-%d", pwgen.AlphaNum(32), i)
		for j := 0; j < countPerNamespace; j++ {
			obj := NewFakeObject(fmt.Sprintf("fake-%d", j), namespace)
			obj.CreationTimestamp = metav1.NewTime(fakeClock.Now())
			obj.Phase = phase
			obj.RefObject = namespace
			Expect(client.Create(context.Background(), obj)).To(Succeed())
		}
	}
}

func newFakeQueue() *fakeQueue {
	limiter := workqueue.DefaultTypedControllerRateLimiter[reconcile.Request]()
	queue := workqueue.NewTypedRateLimitingQueueWithConfig(limiter, workqueue.TypedRateLimitingQueueConfig[reconcile.Request]{Name: "test"})
	return &fakeQueue{
		TypedRateLimitingInterface: queue,
	}
}

type fakeQueue struct {
	requests []reconcile.Request
	workqueue.TypedRateLimitingInterface[reconcile.Request]
}

func (q *fakeQueue) Add(req reconcile.Request) {
	q.requests = append(q.requests, req)
}

func (q *fakeQueue) Requests() []reconcile.Request {
	return q.requests
}

func newScheme() *apiruntime.Scheme {
	GinkgoHelper()

	scheme := apiruntime.NewScheme()
	for _, f := range []func(*apiruntime.Scheme) error{
		clientgoscheme.AddToScheme,
		AddToScheme,
	} {
		Expect(f(scheme)).To(Succeed())
	}
	return scheme
}
