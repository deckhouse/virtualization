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

package framework

import (
	"context"
	"fmt"
	"maps"
	"regexp"
	"slices"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization/test/e2e/internal/framework/failfast"
)

const (
	// ModuleNamespace hosts the virtualization control plane (virt-controller,
	// virt-handler, CDI, ...) whose health every spec depends on.
	ModuleNamespace = "d8-virtualization"

	NamespaceBasePrefix = "v12n-e2e"
	// A label allows to tag the resources created during e2e testing.
	// In case the resource cleanup at the end of the test does not work properly,
	// the resources created during testing can be manually deleted using this label.
	E2ELabel = "v12n-e2e"
)

// Ginkgo spec labels that opt a spec out of a fail-fast rule. They are the
// declarative counterparts of TolerateUnschedulablePods and
// TolerateFailedMigrations, for specs whose framework is created by a shared
// BeforeEach that calls Before for every spec of the container.
const (
	LabelTolerateUnschedulablePods = "tolerate-unschedulable-pods"
	LabelTolerateFailedMigrations  = "tolerate-failed-migrations"
)

type Framework struct {
	Clients

	skipNsCreation  bool
	skipNsDeletion  bool
	namespacePrefix string
	namespace       *corev1.Namespace

	tolerateUnschedulablePods bool
	tolerateFailedMigrations  bool
	failFastExemptions        *failfast.Exemptions

	objectsToDelete []client.Object
}

func NewFramework(namespacePrefix string) *Framework {
	return &Framework{
		Clients:            GetClients(),
		namespacePrefix:    namespacePrefix,
		skipNsCreation:     namespacePrefix == "",
		failFastExemptions: &failfast.Exemptions{},
	}
}

func (f *Framework) Before() {
	GinkgoHelper()

	labels := CurrentSpecReport().Labels()
	if slices.Contains(labels, LabelTolerateUnschedulablePods) {
		f.tolerateUnschedulablePods = true
	}
	if slices.Contains(labels, LabelTolerateFailedMigrations) {
		f.tolerateFailedMigrations = true
	}

	if !f.skipNsCreation {
		ns, err := f.createNamespace(f.namespacePrefix)
		Expect(err).NotTo(HaveOccurred())

		By(fmt.Sprintf("Namespace %q has been created", ns.Name))
		f.namespace = ns
		f.startFailFast()
		f.startLiveCollect()
	}
}

func (f *Framework) After(ctx context.Context) {
	GinkgoHelper()

	if CurrentSpecReport().Failed() {
		if f.namespace != nil {
			By("Failed: save resource dump")
			dumpCtx, cancel := dumpContext(ctx)
			f.saveTestCaseDump(dumpCtx)
			cancel()
		}
	}

	if GetConfig().IsCleanupNeeded() {
		By("Cleanup: process deferred deletions and delete namespace")
		// Delete sends all deletion requests before it starts waiting. The
		// namespace must be part of the same call: on interrupt (Ctrl+C) this
		// cleanup node only has ginkgo's grace period before ctx is cancelled,
		// and a namespace deletion queued behind waiting for the deferred
		// objects never gets requested, leaving Active namespaces behind.
		objs := append([]client.Object{}, f.objectsToDelete...)
		if f.namespace != nil && !f.skipNsDeletion {
			objs = append(objs, f.namespace)
		}
		err := f.Delete(ctx, objs...)
		Expect(err).NotTo(HaveOccurred(), "Failed to delete resources")
	}
}

// dumpContext caps the dump at 80% of the time left before the ctx deadline,
// reserving the rest for resource deletion. On interrupt the whole cleanup
// node lives within ginkgo's grace period: without this cap the dump eats the
// entire budget and Delete never gets to send its requests.
func dumpContext(ctx context.Context) (context.Context, context.CancelFunc) {
	deadline, ok := ctx.Deadline()
	if !ok {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, time.Until(deadline)*4/5)
}

func (f *Framework) createNamespace(prefix string) (*corev1.Namespace, error) {
	ns := &corev1.Namespace{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Namespace",
			APIVersion: corev1.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: fmt.Sprintf("%s-%s-", NamespaceBasePrefix, prefix),
			Labels: map[string]string{
				E2ELabel: "true",
			},
		},
	}

	ns, err := f.KubeClient().CoreV1().Namespaces().Create(context.Background(), ns, metav1.CreateOptions{})
	if err != nil {
		return nil, err
	}

	return ns, nil
}

func (f *Framework) Namespace() *corev1.Namespace {
	return f.namespace
}

// TolerateUnschedulablePods disables the unschedulable-pod fail-fast for the
// spec's namespace, for specs that deliberately park a pod Unschedulable
// (e.g. a nodeSelector pointing at a non-existent node). The other fail-fast
// rules stay active. Must be called before Before; a spec sharing its
// framework setup with others uses LabelTolerateUnschedulablePods instead.
func (f *Framework) TolerateUnschedulablePods() {
	f.tolerateUnschedulablePods = true
}

// TolerateFailedMigrations disables the failed-migration fail-fast rules for
// the spec's namespace, for specs that deliberately cancel, revert or fail a
// live migration. Must be called before Before; a spec sharing its framework
// setup with others uses LabelTolerateFailedMigrations instead.
func (f *Framework) TolerateFailedMigrations() {
	f.tolerateFailedMigrations = true
}

// ExpectFailure exempts the objects from every fail-fast rule of this
// framework, together with the pods they own (an importer or uploader pod
// crash-looping after the expected failure is part of the same picture), for
// specs that deliberately drive a resource into a dead-end state (e.g. an
// image with a spoiled checksum). Call it before creating the object, so the
// exemption is in force by the time the rules first observe it; the expected
// failure is then asserted on the spec's own observer. Unlike the Tolerate*
// helpers, the exemption is per object: every rule stays active for
// everything else in the namespace.
func (f *Framework) ExpectFailure(objs ...client.Object) {
	for _, obj := range objs {
		f.failFastExemptions.Add(obj)
	}
}

// startFailFast launches the fail-fast rules for the spec: pods, block
// devices, VMs and VMOPs of the test namespace, plus the cluster nodes and
// the module's control-plane pods, none of which can wedge without dooming
// the spec. Every rule fails the spec as soon as its subject has been in a
// dead-end state for longer than the finding's grace period. The rules are
// released by DeferCleanup before the framework's own cleanup runs.
func (f *Framework) startFailFast() {
	if f.namespace == nil {
		return
	}

	ctx, cancel := context.WithCancel(context.Background())
	DeferCleanup(cancel)

	ns := f.namespace.Name
	testPods := f.KubeClient().CoreV1().Pods(ns)
	modulePods := f.KubeClient().CoreV1().Pods(ModuleNamespace)

	var rules []failfast.FailFast

	if !f.tolerateUnschedulablePods {
		rules = append(rules, failfast.Unschedulable(testPods, ns))
	}
	if !f.tolerateFailedMigrations {
		rules = append(rules,
			failfast.KVVMIMigrationFailed(f.DynamicClient(), ns),
			failfast.VirtualDiskMigrationReverted(f.VirtClient().VirtualDisks(ns), ns),
			failfast.VMOPFailed(f.VirtClient().VirtualMachineOperations(ns), ns),
		)
	}

	rules = append(
		rules,
		failfast.ImagePull(testPods, ns),
		failfast.CrashLoop(testPods, ns),
		failfast.Unschedulable(modulePods, ModuleNamespace),
		failfast.ImagePull(modulePods, ModuleNamespace),
		failfast.CrashLoop(modulePods, ModuleNamespace),
		failfast.NodeNotReady(f.KubeClient().CoreV1().Nodes()),
		failfast.VirtualDisks(f.VirtClient().VirtualDisks(ns), ns),
		failfast.VirtualImages(f.VirtClient().VirtualImages(ns), ns),
		failfast.VMSyncDeniedByWebhook(f.VirtClient().VirtualMachines(ns), ns),
		failfast.VMOPStuckPending(f.VirtClient().VirtualMachineOperations(ns), ns),
	)

	for _, rule := range rules {
		rule.Start(ctx, f.failFastExemptions)
	}
}

// SetProjectNamespace makes the framework operate inside an externally managed namespace,
// such as the one created by a Deckhouse Project. After does not delete this namespace:
// the owning resource (e.g. the Project) is expected to be cleaned up separately (for
// instance via CreateWithDeferredDeletion).
func (f *Framework) SetProjectNamespace(name string) {
	f.namespace = &corev1.Namespace{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Namespace",
			APIVersion: corev1.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{Name: name},
	}
	f.skipNsDeletion = true
	f.startFailFast()
	f.startLiveCollect()
}

func (f *Framework) DeferDelete(objs ...client.Object) {
	f.objectsToDelete = append(f.objectsToDelete, objs...)
}

func (f *Framework) Delete(ctx context.Context, objs ...client.Object) error {
	// 1. Send deletion request for objects.
	for _, obj := range objs {
		err := f.client.Delete(ctx, obj)
		if err != nil && !k8serrors.IsNotFound(err) {
			return err
		}
	}

	// 2. Wait for the objects to be deleted.
	for _, obj := range objs {
		key := types.NamespacedName{
			Namespace: obj.GetNamespace(),
			Name:      obj.GetName(),
		}

		err := wait.PollUntilContextTimeout(ctx, time.Second, LongTimeout, true, func(ctx context.Context) (bool, error) {
			err := f.client.Get(ctx, key, obj)
			switch {
			case err == nil:
				return false, nil
			case k8serrors.IsNotFound(err):
				return true, nil
			default:
				return false, err
			}
		})
		if err != nil {
			return fmt.Errorf("object %q not deleted in time: %w", key, err)
		}
	}

	return nil
}

// CreateWithDeferredDeletion creates one or more Kubernetes resources and
// adds them to a list for deferred deletion.
//
// Returns an error if the creation of any resource
func (f *Framework) CreateWithDeferredDeletion(ctx context.Context, objs ...client.Object) error {
	for _, obj := range objs {
		labels := obj.GetLabels()
		if labels == nil {
			labels = make(map[string]string)
		}
		maps.Copy(labels, map[string]string{E2ELabel: f.namespacePrefix})
		obj.SetLabels(labels)

		err := f.client.Create(ctx, obj)
		if err != nil {
			SkipIfKnownClusterWebhookFailure(err)
			return err
		}
		f.DeferDelete(obj)
	}

	return nil
}

// knownFlakyClusterWebhookRe matches create failures caused by known-flaky cluster
// webhooks on the dev cluster: the multitenancy-manager pod intermittently dies with
// "fatal error: concurrent map writes" under parallel Project creation, and the
// sds-replicated-volume PVC validation webhook transiently refuses connections. While
// they restart, the apiserver reports EOF / connection errors for every affected create.
var knownFlakyClusterWebhookRe = regexp.MustCompile(
	`failed calling webhook "(projects\.multitenancy-webhook\.deckhouse\.io|d8-sds-replicated-volume-pvc-validation\.deckhouse\.io)"`)

// TODO: remove when the multitenancy-manager crash and the sds-replicated-volume
// webhook connectivity flake are fixed in Deckhouse.
func SkipIfKnownClusterWebhookFailure(err error) {
	GinkgoHelper()

	if err == nil {
		return
	}
	if knownFlakyClusterWebhookRe.MatchString(err.Error()) {
		Skip("skip due to known flaky cluster webhook failure: " + err.Error())
	}
}
