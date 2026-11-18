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

package peering

import (
	"context"
	"encoding/base64"
	"fmt"
	"maps"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	authnv1 "k8s.io/api/authentication/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/uuid"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/builder/vd"
	"github.com/deckhouse/virtualization-controller/pkg/builder/vm"
	"github.com/deckhouse/virtualization-controller/pkg/builder/vmip"
	vmopbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vmop"
	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	commonvmop "github.com/deckhouse/virtualization-controller/pkg/common/vmop"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmopcondition"
	intvirt "github.com/deckhouse/virtualization/api/internalvirtualization/v1alpha1"
	"github.com/deckhouse/virtualization/test/e2e/eventually"
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
	"github.com/deckhouse/virtualization/test/e2e/internal/object"
	vmobs "github.com/deckhouse/virtualization/test/e2e/internal/observer/vm"
	"github.com/deckhouse/virtualization/test/e2e/internal/rewrite"
)

const (
	PeerCleanupTimeout = 15 * time.Second
	peerCleanupStable  = 3 * time.Second
	peerActivePolling  = 100 * time.Millisecond
	peerMarker         = "/root/peering-marker"
)

// PeerMigration exercises ordinary VMOPs against another namespace of this cluster.
// Addresses are reserved through VMIPAddress, so concurrent tests cannot take them.
type PeerMigration struct {
	Context                context.Context
	Source, Target         *framework.Framework
	Peer                   string
	VM                     *v1alpha2.VirtualMachine
	Disk                   *v1alpha2.VirtualDisk
	bootID, serial, marker string
}

type PeerRuntime struct {
	InstanceUID types.UID
	ActivePods  map[types.UID]string
	Pods        map[string]corev1.Pod
}

func NewPeerMigration() *PeerMigration {
	GinkgoHelper()
	// A BeforeEach SpecContext expires when that node ends; this context must
	// cover the test body and both namespaces' cleanup as well.
	ctx, cancel := context.WithCancel(context.Background())
	DeferCleanup(cancel)

	h := &PeerMigration{Context: ctx, Source: framework.NewFramework("peering-src"), Target: framework.NewFramework("peering-dst")}
	DeferCleanup(h.Source.After)
	h.Source.Before()
	DeferCleanup(h.Target.After)
	h.Target.Before()
	h.Peer = h.Target.Namespace().Name
	secret, err := NewSelfPeerSecret(ctx, h.Source, h.Peer, h.Source.Namespace().Name, h.Target.Namespace().Name)
	Expect(err).NotTo(HaveOccurred())
	Expect(h.Source.GenericClient().Create(ctx, secret)).To(Succeed())
	h.Source.DeferDelete(secret)
	h.Disk = object.NewVDFromCVI("vd-root", h.Source.Namespace().Name, object.PrecreatedCVICustomBIOS, vd.WithSize(ptr.To(resource.MustParse("2Gi"))))
	h.VM = object.NewMinimalVM("vm-peer-", h.Source.Namespace().Name, vm.WithBootloader(v1alpha2.BIOS), vm.WithDisks(h.Disk), vm.WithCPU(1, ptr.To("20%")), vm.WithMemory(resource.MustParse("256Mi")))

	return h
}

func (h *PeerMigration) Start() {
	GinkgoHelper()
	Expect(h.Source.CreateWithDeferredDeletion(h.Context, h.Disk, h.VM)).To(Succeed())
	// The receiver creates these objects; they must be deleted before the
	// reserved IP and namespace are finalized, including after a failed test.
	h.Target.DeferDelete(
		&v1alpha2.VirtualMachine{ObjectMeta: metav1.ObjectMeta{Name: h.VM.Name, Namespace: h.Target.Namespace().Name}},
		&v1alpha2.VirtualDisk{ObjectMeta: metav1.ObjectMeta{Name: h.Disk.Name, Namespace: h.Target.Namespace().Name}},
	)
	obs := vmobs.StartObserver(h.Context, h.Source, h.VM)
	obs.Never(vmobs.BeFailed())
	Expect(obs.WaitFor(vmobs.BeRunning(), framework.LongTimeout)).To(Succeed())
	eventually.SSHReadyAsRoot(h.Source, h.VM, framework.LongTimeout)
	h.bootID, h.serial = PeerGuestIdentity(h.Source, h.VM)
	h.marker = h.Source.Namespace().Name
	_, err := h.Source.SSHCommand(h.VM.Name, h.VM.Namespace, fmt.Sprintf("echo %s > %s && sync", h.marker, peerMarker), framework.WithSSHUser("root"))
	Expect(err).NotTo(HaveOccurred())
}

func (h *PeerMigration) ReserveAddress(f *framework.Framework) *v1alpha2.VirtualMachineIPAddress {
	GinkgoHelper()

	address := object.NewVirtualMachineIPAddress("peer-reserved-", f.Namespace().Name, vmip.WithTypeAuto())
	Expect(f.CreateWithDeferredDeletion(h.Context, address)).To(Succeed())
	Eventually(func(g Gomega) {
		g.Expect(f.GenericClient().Get(h.Context, client.ObjectKeyFromObject(address), address)).To(Succeed())
		g.Expect(address.Status.Phase).To(Equal(v1alpha2.VirtualMachineIPAddressPhaseBound))
		g.Expect(address.Status.Address).NotTo(BeEmpty())
		g.Expect(address.Status.VirtualMachine).To(BeEmpty())
	}).WithTimeout(framework.MiddleTimeout).WithPolling(framework.PollingInterval).Should(Succeed(), "reserve an unused address in %s", address.Namespace)

	return address
}

func (h *PeerMigration) Operation(from *framework.Framework, address *v1alpha2.VirtualMachineIPAddress) *v1alpha2.VirtualMachineOperation {
	op := vmopbuilder.New(vmopbuilder.WithName("vmop-peer-"+string(uuid.NewUUID())), vmopbuilder.WithNamespace(from.Namespace().Name), vmopbuilder.WithVirtualMachine(h.VM.Name), vmopbuilder.WithType(v1alpha2.VMOPTypeMigrate), vmopbuilder.WithAnnotation(annotations.AnnVMOPNewIPAddress, address.Status.Address))
	op.Spec.Migrate = &v1alpha2.VirtualMachineOperationMigrateSpec{ProjectPeerName: h.Peer}

	return op
}

func (h *PeerMigration) Create(from *framework.Framework, op *v1alpha2.VirtualMachineOperation, expectedFailure bool) {
	GinkgoHelper()

	if expectedFailure {
		from.ExpectFailure(op)
	}

	Expect(from.CreateWithDeferredDeletion(h.Context, op)).To(Succeed(), "create peer migration %s/%s", op.Namespace, op.Name)
}

func (h *PeerMigration) WaitPhase(op *v1alpha2.VirtualMachineOperation, phase v1alpha2.VMOPPhase, timeout time.Duration) {
	GinkgoHelper()
	Eventually(func(g Gomega) {
		g.Expect(h.Source.GenericClient().Get(h.Context, client.ObjectKeyFromObject(op), op)).To(Succeed())

		if op.Status.Phase != phase && (op.Status.Phase == v1alpha2.VMOPPhaseCompleted || op.Status.Phase == v1alpha2.VMOPPhaseFailed) {
			StopTrying(fmt.Sprintf("%s/%s ended %s instead of %s: %v", op.Namespace, op.Name, op.Status.Phase, phase, op.Status.Conditions)).Now()
		}

		g.Expect(op.Status.Phase).To(Equal(phase), "conditions: %v", op.Status.Conditions)
	}).WithTimeout(timeout).WithPolling(framework.PollingInterval).Should(Succeed())
}

func (h *PeerMigration) WaitPreparing(op *v1alpha2.VirtualMachineOperation) {
	GinkgoHelper()
	Eventually(func(g Gomega) {
		g.Expect(h.Source.GenericClient().Get(h.Context, client.ObjectKeyFromObject(op), op)).To(Succeed())
		g.Expect(op.Status.Phase).To(Equal(v1alpha2.VMOPPhaseInProgress))
		condition := meta.FindStatusCondition(op.Status.Conditions, string(vmopcondition.TypeCompleted))
		g.Expect(condition).NotTo(BeNil())
		g.Expect(condition.Reason).To(Equal(string(vmopcondition.ReasonWaitingForPeer)))

		request := &intvirt.InternalVirtualizationVirtualMachineReceiver{}
		g.Expect(h.Target.GenericClient().Get(h.Context, client.ObjectKey{Namespace: h.Target.Namespace().Name, Name: h.VM.Name}, request)).To(Succeed())
	}).WithTimeout(framework.MiddleTimeout).WithPolling(peerActivePolling).Should(Succeed(), "observe a request being prepared")
}

// WaitCopying follows runtime state: public VMOP conditions can lag behind a short copy.
func (h *PeerMigration) WaitCopying(op *v1alpha2.VirtualMachineOperation) {
	GinkgoHelper()
	Eventually(func(g Gomega) {
		g.Expect(h.Source.GenericClient().Get(h.Context, client.ObjectKeyFromObject(op), op)).To(Succeed())

		if op.Status.Phase == v1alpha2.VMOPPhaseCompleted || op.Status.Phase == v1alpha2.VMOPPhaseFailed {
			StopTrying(fmt.Sprintf("%s/%s ended before fault injection: %v", op.Namespace, op.Name, op.Status.Conditions)).Now()
		}

		migration := &rewrite.VirtualMachineInstanceMigration{}
		g.Expect(h.Source.RewriteClient().Get(h.Context, commonvmop.MigrationName(op.Name), migration, rewrite.InNamespace(op.Namespace))).To(Succeed())

		instance := &rewrite.VirtualMachineInstance{}
		g.Expect(h.Source.RewriteClient().Get(h.Context, h.VM.Name, instance, rewrite.InNamespace(op.Namespace))).To(Succeed())
		state := instance.Status.MigrationState
		g.Expect(state).NotTo(BeNil())
		g.Expect(state.MigrationUID).To(Equal(migration.UID), "instance state must belong to VMOP %s/%s", op.Namespace, op.Name)

		if state.Completed || state.Failed {
			StopTrying("the migration ended before fault injection").Now()
		}

		g.Expect(state.StartTimestamp).NotTo(BeNil())
		g.Expect(op.Status.Phase).To(Equal(v1alpha2.VMOPPhaseInProgress))
	}).WithTimeout(framework.LongTimeout).WithPolling(peerActivePolling).Should(Succeed(), "observe active native migration for %s", h.VM.Name)
}

func (h *PeerMigration) Runtime() PeerRuntime {
	GinkgoHelper()

	instance := &rewrite.VirtualMachineInstance{}
	Expect(h.Source.RewriteClient().Get(h.Context, h.VM.Name, instance, rewrite.InNamespace(h.VM.Namespace))).To(Succeed())
	state := PeerRuntime{InstanceUID: instance.UID, ActivePods: maps.Clone(instance.Status.ActivePods), Pods: map[string]corev1.Pod{}}
	pods, err := h.Source.KubeClient().CoreV1().Pods(h.VM.Namespace).List(h.Context, metav1.ListOptions{})
	Expect(err).NotTo(HaveOccurred())

	for _, pod := range pods.Items {
		if _, ok := state.ActivePods[pod.UID]; ok {
			state.Pods[pod.Name] = pod
		}
	}

	Expect(state.Pods).NotTo(BeEmpty(), "source launcher pod was identified")

	return state
}

func (h *PeerMigration) AssertSource(runtime PeerRuntime) {
	GinkgoHelper()
	Eventually(func(g Gomega) {
		vm := &v1alpha2.VirtualMachine{}
		g.Expect(h.Source.GenericClient().Get(h.Context, client.ObjectKeyFromObject(h.VM), vm)).To(Succeed())
		g.Expect(vm.Status.Phase).To(Equal(v1alpha2.MachineRunning))
		g.Expect(vm.Annotations).NotTo(HaveKey("virtualization.deckhouse.io/moved-to-peer"))

		instance := &rewrite.VirtualMachineInstance{}
		g.Expect(h.Source.RewriteClient().Get(h.Context, h.VM.Name, instance, rewrite.InNamespace(h.VM.Namespace))).To(Succeed())
		g.Expect(instance.UID).To(Equal(runtime.InstanceUID))
		g.Expect(instance.Status.ActivePods).To(Equal(runtime.ActivePods))

		for name, old := range runtime.Pods {
			pod, err := h.Source.KubeClient().CoreV1().Pods(h.VM.Namespace).Get(h.Context, name, metav1.GetOptions{})
			g.Expect(err).NotTo(HaveOccurred())
			g.Expect(pod.UID).To(Equal(old.UID))

			restarts := map[string]int32{}
			for _, container := range old.Status.ContainerStatuses {
				restarts[container.Name] = container.RestartCount
			}

			g.Expect(pod.Status.ContainerStatuses).To(HaveLen(len(restarts)))

			for _, container := range pod.Status.ContainerStatuses {
				g.Expect(restarts).To(HaveKeyWithValue(container.Name, container.RestartCount))
			}
		}
	}).WithTimeout(PeerCleanupTimeout).WithPolling(framework.PollingInterval).Should(Succeed(), "source runtime is unchanged and unlocked")

	kvvm := &rewrite.VirtualMachine{}
	Expect(h.Source.RewriteClient().Get(h.Context, h.VM.Name, kvvm, rewrite.InNamespace(h.VM.Namespace))).To(Succeed())
	Expect(kvvm.Annotations).NotTo(HaveKey(annotations.AnnPeerMigrationVMOP))
}

func (h *PeerMigration) AssertRollbackCleanup(address *v1alpha2.VirtualMachineIPAddress) {
	GinkgoHelper()

	check := func(g Gomega) {
		for _, obj := range []client.Object{&v1alpha2.VirtualMachine{}, &v1alpha2.VirtualDisk{}, &intvirt.InternalVirtualizationVirtualMachineReceiver{}} {
			name := h.VM.Name
			if _, ok := obj.(*v1alpha2.VirtualDisk); ok {
				name = h.Disk.Name
			}

			obj.SetName(name)
			obj.SetNamespace(h.Target.Namespace().Name)
			h.expectRemoved(g, h.Target, obj)
		}

		instance := &rewrite.VirtualMachineInstance{}
		err := h.Target.RewriteClient().Get(h.Context, h.VM.Name, instance, rewrite.InNamespace(h.Target.Namespace().Name))
		expectMissing(g, instance.VirtualMachineInstance, err)

		kvvm := &rewrite.VirtualMachine{}
		err = h.Target.RewriteClient().Get(h.Context, h.VM.Name, kvvm, rewrite.InNamespace(h.Target.Namespace().Name))
		expectMissing(g, kvvm.VirtualMachine, err)

		pvcs := &corev1.PersistentVolumeClaimList{}
		g.Expect(h.Target.GenericClient().List(h.Context, pvcs, client.InNamespace(h.Target.Namespace().Name))).To(Succeed())
		g.Expect(pvcs.Items).To(BeEmpty(), "no PVC is orphaned after its VirtualDisk is gone")

		ips := &v1alpha2.VirtualMachineIPAddressList{}
		g.Expect(h.Target.GenericClient().List(h.Context, ips, client.InNamespace(h.Target.Namespace().Name))).To(Succeed())
		g.Expect(ips.Items).To(HaveLen(1), "only the test's preallocated IP remains")
		g.Expect(ips.Items[0].UID).To(Equal(address.UID))
		g.Expect(ips.Items[0].Status.VirtualMachine).To(BeEmpty())

		migrations := &rewrite.VirtualMachineInstanceMigrationList{}
		g.Expect(h.Target.RewriteClient().List(h.Context, migrations, rewrite.InNamespace(h.Target.Namespace().Name))).To(Succeed())
		g.Expect(migrations.Items).To(BeEmpty(), "target migrations are removed after a failed receive")

		sourceMigrations := &rewrite.VirtualMachineInstanceMigrationList{}
		g.Expect(h.Source.RewriteClient().List(h.Context, sourceMigrations, rewrite.InNamespace(h.Source.Namespace().Name))).To(Succeed())
		g.Expect(sourceMigrations.Items).To(BeEmpty(), "source migrations must not block another VMOP")
	}
	Eventually(check).WithTimeout(PeerCleanupTimeout).WithPolling(framework.PollingInterval).Should(Succeed())
	Consistently(check).WithTimeout(peerCleanupStable).WithPolling(framework.PollingInterval).Should(Succeed(), "cleanup remains complete after controller updates")
}

func (h *PeerMigration) CancelAndVerify(op *v1alpha2.VirtualMachineOperation, address *v1alpha2.VirtualMachineIPAddress, runtime PeerRuntime) {
	GinkgoHelper()
	Expect(h.Source.GenericClient().Delete(h.Context, op)).To(Succeed())
	Eventually(func(g Gomega) {
		h.expectRemoved(g, h.Source, &v1alpha2.VirtualMachineOperation{ObjectMeta: metav1.ObjectMeta{Name: op.Name, Namespace: op.Namespace}})
	}).WithTimeout(framework.MiddleTimeout).WithPolling(framework.PollingInterval).Should(Succeed(), "cancelled VMOP is removed")
	h.AssertRollbackCleanup(address)
	h.AssertSource(runtime)
}

func (h *PeerMigration) RenewAddress(from *framework.Framework) {
	GinkgoHelper()

	script := `old=$(ip -4 addr show dev eth0 | grep inet)
while :; do
 cur=$(ip -4 addr show dev eth0 | grep inet)
 [ -n "$cur" ] && [ "$cur" != "$old" ] && break
 kill -USR1 $(cat /var/run/udhcpc.eth0.pid)
 sleep 5
done
`
	_, err := from.SSHCommand(h.VM.Name, from.Namespace().Name, fmt.Sprintf("echo %s | base64 -d > /root/peer-renew.sh && (nohup sh /root/peer-renew.sh >/dev/null 2>&1 </dev/null &)", base64.StdEncoding.EncodeToString([]byte(script))), framework.WithSSHUser("root"))
	Expect(err).NotTo(HaveOccurred())
}

// RenewUntil keeps DHCP renewal alive across both directions of an immediate round trip.
func (h *PeerMigration) RenewUntil(from *framework.Framework, address string) {
	GinkgoHelper()

	script := fmt.Sprintf(`while ! ip -4 addr show dev eth0 | grep -Fq " %s/"; do
 kill -USR1 $(cat /var/run/udhcpc.eth0.pid)
 sleep 1
done
`, address)
	_, err := from.SSHCommand(h.VM.Name, from.Namespace().Name, fmt.Sprintf("echo %s | base64 -d > /root/peer-roundtrip.sh && (nohup sh /root/peer-roundtrip.sh >/dev/null 2>&1 </dev/null &)", base64.StdEncoding.EncodeToString([]byte(script))), framework.WithSSHUser("root"))
	Expect(err).NotTo(HaveOccurred())
}

func (h *PeerMigration) AssertGuest(to *framework.Framework, address *v1alpha2.VirtualMachineIPAddress) {
	GinkgoHelper()

	moved := &v1alpha2.VirtualMachine{ObjectMeta: metav1.ObjectMeta{Name: h.VM.Name, Namespace: to.Namespace().Name}}

	Eventually(func(g Gomega) {
		g.Expect(to.GenericClient().Get(h.Context, client.ObjectKeyFromObject(moved), moved)).To(Succeed())
		g.Expect(moved.Status.Phase).To(Equal(v1alpha2.MachineRunning))
		g.Expect(moved.Status.IPAddress).To(Equal(address.Status.Address))
		g.Expect(moved.Spec.VirtualMachineIPAddress).To(Equal(address.Name))
	}).WithTimeout(framework.MiddleTimeout).WithPolling(framework.PollingInterval).Should(Succeed())

	current := &v1alpha2.VirtualMachineIPAddress{}
	Expect(to.GenericClient().Get(h.Context, client.ObjectKeyFromObject(address), current)).To(Succeed())
	Expect(current.UID).To(Equal(address.UID), "the reserved VMIPAddress is reused, not recreated")
	eventually.SSHReadyAsRoot(to, moved, framework.LongTimeout)
	boot, serial := PeerGuestIdentity(to, moved)
	Expect(boot).To(Equal(h.bootID))
	Expect(serial).To(Equal(h.serial))

	out, err := to.SSHCommand(moved.Name, moved.Namespace, fmt.Sprintf("sync && echo 3 > /proc/sys/vm/drop_caches && cat %s", peerMarker), framework.WithSSHUser("root"))
	Expect(err).NotTo(HaveOccurred())
	Expect(strings.TrimSpace(out)).To(Equal(h.marker))
}

func (h *PeerMigration) AssertMigrationCleanup(from, to *framework.Framework) {
	GinkgoHelper()
	Eventually(func(g Gomega) {
		for _, obj := range []client.Object{
			&v1alpha2.VirtualMachine{ObjectMeta: metav1.ObjectMeta{Name: h.VM.Name, Namespace: from.Namespace().Name}},
			&v1alpha2.VirtualDisk{ObjectMeta: metav1.ObjectMeta{Name: h.Disk.Name, Namespace: from.Namespace().Name}},
		} {
			h.expectRemoved(g, from, obj)
		}

		h.expectRemoved(g, to, &intvirt.InternalVirtualizationVirtualMachineReceiver{ObjectMeta: metav1.ObjectMeta{Name: h.VM.Name, Namespace: to.Namespace().Name}})
	}).WithTimeout(PeerCleanupTimeout).WithPolling(framework.PollingInterval).Should(Succeed(), "source resources and the receive request are removed")
}

func (h *PeerMigration) expectRemoved(g Gomega, f *framework.Framework, obj client.Object) {
	GinkgoHelper()

	err := f.GenericClient().Get(h.Context, client.ObjectKeyFromObject(obj), obj)
	expectMissing(g, obj, err)
}

func expectMissing(g Gomega, obj client.Object, err error) {
	GinkgoHelper()
	g.Expect(client.IgnoreNotFound(err)).To(Succeed(), "read %T before checking removal", obj)

	if apierrors.IsNotFound(err) {
		return
	}

	data, conversionErr := runtime.DefaultUnstructuredConverter.ToUnstructured(obj)
	g.Expect(conversionErr).NotTo(HaveOccurred())

	phase, _, _ := unstructured.NestedString(data, "status", "phase")
	g.Expect(apierrors.IsNotFound(err)).To(BeTrue(), "%T %s/%s remains: phase=%q, deletionTimestamp=%v, finalizers=%v",
		obj, obj.GetNamespace(), obj.GetName(), phase, obj.GetDeletionTimestamp(), obj.GetFinalizers())
}

func (h *PeerMigration) MigrateAndVerify(address *v1alpha2.VirtualMachineIPAddress) {
	GinkgoHelper()
	h.RenewAddress(h.Source)
	op := h.Operation(h.Source, address)
	h.Create(h.Source, op, false)
	h.WaitPhase(op, v1alpha2.VMOPPhaseCompleted, framework.MaxTimeout)
	h.AssertMigrationCleanup(h.Source, h.Target)
	h.AssertGuest(h.Target, address)
	h.AssertTerminal(op, v1alpha2.VMOPPhaseCompleted)
}

func (h *PeerMigration) AssertTerminal(op *v1alpha2.VirtualMachineOperation, phase v1alpha2.VMOPPhase) {
	GinkgoHelper()
	Consistently(func(g Gomega) {
		current := &v1alpha2.VirtualMachineOperation{}
		g.Expect(h.Source.GenericClient().Get(h.Context, client.ObjectKeyFromObject(op), current)).To(Succeed())
		g.Expect(current.Status.Phase).To(Equal(phase))
	}).WithTimeout(peerCleanupStable).WithPolling(framework.PollingInterval).Should(Succeed(), "VMOP terminal phase never changes")
}

// PeerGuestIdentity reads SCSI page 0x80 because the custom guest has no udev serial database.
func PeerGuestIdentity(f *framework.Framework, vm *v1alpha2.VirtualMachine) (string, string) {
	GinkgoHelper()

	out, err := f.SSHCommand(vm.Name, vm.Namespace, "cat /proc/sys/kernel/random/boot_id && tail -c +5 /sys/block/sda/device/vpd_pg80", framework.WithSSHUser("root"))
	Expect(err).NotTo(HaveOccurred())

	fields := strings.Fields(out)
	Expect(fields).To(HaveLen(2), "boot ID and root disk serial: %q", out)

	return fields[0], fields[1]
}

func NewSelfPeerSecret(ctx context.Context, f *framework.Framework, peer, from, to string) (*corev1.Secret, error) {
	token, err := f.KubeClient().CoreV1().ServiceAccounts(framework.ModuleNamespace).CreateToken(ctx, "virtualization-controller", &authnv1.TokenRequest{Spec: authnv1.TokenRequestSpec{ExpirationSeconds: ptr.To[int64](3600)}}, metav1.CreateOptions{})
	if err != nil {
		return nil, fmt.Errorf("create controller token: %w", err)
	}

	root, err := f.KubeClient().CoreV1().ConfigMaps(framework.ModuleNamespace).Get(ctx, "kube-root-ca.crt", metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("read cluster CA: %w", err)
	}

	config := fmt.Sprintf("apiVersion: v1\nkind: Config\nclusters: [{name: self, cluster: {server: \"https://kubernetes.default.svc\", certificate-authority-data: %q}}]\nusers: [{name: self, user: {token: %q}}]\ncontexts: [{name: self, context: {cluster: self, user: self}}]\ncurrent-context: self\n", base64.StdEncoding.EncodeToString([]byte(root.Data["ca.crt"])), token.Status.Token)

	return &corev1.Secret{ObjectMeta: metav1.ObjectMeta{Name: "peering-dev-peer-" + peer, Namespace: framework.ModuleNamespace}, Data: map[string][]byte{"kubeconfig": []byte(config), "namespaces": []byte(from + "=" + to + "," + to + "=" + from)}}, nil
}
