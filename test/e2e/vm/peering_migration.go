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

package vm

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	authnv1 "k8s.io/api/authentication/v1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/builder/vd"
	"github.com/deckhouse/virtualization-controller/pkg/builder/vm"
	vmopbuilder "github.com/deckhouse/virtualization-controller/pkg/builder/vmop"
	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vmopcondition"
	intvirtv1alpha1 "github.com/deckhouse/virtualization/api/internalvirtualization/v1alpha1"
	"github.com/deckhouse/virtualization/test/e2e/eventually"
	"github.com/deckhouse/virtualization/test/e2e/internal/framework"
	"github.com/deckhouse/virtualization/test/e2e/internal/label"
	"github.com/deckhouse/virtualization/test/e2e/internal/object"
	vmobs "github.com/deckhouse/virtualization/test/e2e/internal/observer/vm"
	vmopobs "github.com/deckhouse/virtualization/test/e2e/internal/observer/vmop"
	"github.com/deckhouse/virtualization/test/e2e/internal/precheck"
	"github.com/deckhouse/virtualization/test/e2e/internal/util"
)

// The cluster is its own peer: a dev Secret gives virtualization-controller a kubeconfig of this cluster and
// maps the namespace of the source to another one. The VM gets an address there, as the address of the
// source stays taken in the same cluster.
var _ = Describe("PeeringMigration", Label(label.SIGCompute, precheck.PrecheckPeering), func() {
	var (
		src, dst       *framework.Framework
		ctx            context.Context
		peer           string
		virtualDisk    *v1alpha2.VirtualDisk
		virtualMachine *v1alpha2.VirtualMachine
	)

	BeforeEach(func() {
		ctx = context.Background()
		src = framework.NewFramework("peering-src")
		DeferCleanup(src.After)
		src.Before()
		dst = framework.NewFramework("peering-dst")
		DeferCleanup(dst.After)
		dst.Before()
		peer = dst.Namespace().Name

		secret, err := newSelfPeerSecret(ctx, src, peer, src.Namespace().Name, dst.Namespace().Name)
		Expect(err).NotTo(HaveOccurred())
		Expect(src.GenericClient().Create(ctx, secret)).To(Succeed())
		src.DeferDelete(secret)

		virtualDisk = object.NewVDFromCVI("vd-root", src.Namespace().Name, object.PrecreatedCVICustomBIOS,
			vd.WithSize(ptr.To(resource.MustParse(vdCustomImageSize))))
		virtualMachine = object.NewMinimalVM("vm-", src.Namespace().Name, vm.WithBootloader(v1alpha2.BIOS), vm.WithDisks(virtualDisk))
	})

	runVM := func() {
		GinkgoHelper()
		Expect(src.CreateWithDeferredDeletion(ctx, virtualDisk, virtualMachine)).To(Succeed())
		vmObs := vmobs.StartObserver(ctx, src, virtualMachine)
		vmObs.Never(vmobs.BeFailed())
		Expect(vmObs.WaitFor(vmobs.BeRunning(), framework.LongTimeout)).To(Succeed())
	}

	move := func() *v1alpha2.VirtualMachineOperation {
		GinkgoHelper()
		vmop := vmopbuilder.New(
			vmopbuilder.WithGenerateName(fmt.Sprintf("%s-move-", util.VmopE2ePrefix)),
			vmopbuilder.WithNamespace(src.Namespace().Name),
			vmopbuilder.WithType(v1alpha2.VMOPTypeMigrate),
			vmopbuilder.WithVirtualMachine(virtualMachine.Name),
		)
		vmop.Spec.Migrate = &v1alpha2.VirtualMachineOperationMigrateSpec{ProjectPeerName: peer}
		Expect(src.CreateWithDeferredDeletion(ctx, vmop)).To(Succeed())
		return vmop
	}

	It("moves a running VM to a peer with a InternalVirtualizationVirtualMachineReceiver", func() {
		var bootID, serial string
		marker := src.Namespace().Name

		By("Run a VM in the source namespace", runVM)

		By("Write a marker to the disk", func() {
			eventually.SSHReadyAsRoot(src, virtualMachine, framework.LongTimeout)
			bootID, serial = guestBootIDAndSerial(src, virtualMachine)
			_, err := src.SSHCommand(virtualMachine.Name, virtualMachine.Namespace,
				fmt.Sprintf("echo %s > %s && sync", marker, peeringMarkerFile), framework.WithSSHUser("root"))
			Expect(err).NotTo(HaveOccurred())
			// The peer gives the VM another address, and a live guest keeps its lease: renew it until the address
			// changes, not after, as every renewal flushes the address and breaks the SSH sessions.
			_, err = src.SSHCommand(virtualMachine.Name, virtualMachine.Namespace,
				fmt.Sprintf("echo %s | base64 -d > /root/renew.sh && (nohup sh /root/renew.sh >/dev/null 2>&1 &)",
					base64.StdEncoding.EncodeToString([]byte(renewUntilAddressChanges))),
				framework.WithSSHUser("root"))
			Expect(err).NotTo(HaveOccurred())
		})

		By("Move the VM to the peer", func() {
			vmop := move()
			vmopObs := vmopobs.StartObserver(ctx, vmop)
			// The peer publishes the address and the key pin of its synchronization controller in the request;
			// without them the source never starts, and the move fails only by a timeout.
			Eventually(func(g Gomega) {
				request := &intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver{}
				g.Expect(dst.GenericClient().Get(ctx, client.ObjectKey{Namespace: dst.Namespace().Name, Name: virtualMachine.Name}, request)).To(Succeed())
				g.Expect(request.Status.SynchronizationAddress).NotTo(BeEmpty(), "the peer published the address of its synchronization controller")
				g.Expect(request.Status.SyncPin).To(HavePrefix("sha256:"), "the peer published the pin of its synchronization key")
			}).WithTimeout(framework.MiddleTimeout).WithPolling(framework.PollingInterval).Should(Succeed())
			Expect(vmopObs.WaitFor(vmopobs.BeCompleted(), framework.MaxTimeout)).To(Succeed())
		})

		By("Check the guest kept running with its disk", func() {
			moved := &v1alpha2.VirtualMachine{}
			Eventually(func(g Gomega) {
				g.Expect(dst.GenericClient().Get(ctx, client.ObjectKey{Namespace: dst.Namespace().Name, Name: virtualMachine.Name}, moved)).To(Succeed())
				g.Expect(moved.Status.Phase).To(Equal(v1alpha2.MachineRunning))
			}).WithTimeout(framework.LongTimeout).WithPolling(framework.PollingInterval).Should(Succeed())

			Eventually(func(g Gomega) {
				out, err := dst.SSHCommand(moved.Name, moved.Namespace, "ip -4 addr show dev eth0",
					framework.WithSSHUser("root"), framework.WithSSHTimeout(5*time.Second))
				g.Expect(err).NotTo(HaveOccurred())
				g.Expect(out).To(ContainSubstring(moved.Status.IPAddress+"/"), "the guest took the address of the peer")
			}).WithTimeout(framework.LongTimeout).WithPolling(framework.PollingInterval).Should(Succeed())
			movedBootID, movedSerial := guestBootIDAndSerial(dst, moved)
			Expect(movedBootID).To(Equal(bootID), "the guest did not reboot")
			Expect(movedSerial).To(Equal(serial), "the guest sees the same disk serial")

			// The page cache came with the memory: only a read past it proves the data is on the new disk.
			out, err := dst.SSHCommand(moved.Name, moved.Namespace,
				fmt.Sprintf("echo 3 > /proc/sys/vm/drop_caches && cat %s", peeringMarkerFile), framework.WithSSHUser("root"))
			Expect(err).NotTo(HaveOccurred())
			Expect(strings.TrimSpace(out)).To(Equal(marker), "the disk data is in the peer")

			disk := &v1alpha2.VirtualDisk{}
			Expect(dst.GenericClient().Get(ctx, client.ObjectKey{Namespace: dst.Namespace().Name, Name: virtualDisk.Name}, disk)).To(Succeed())
			Expect(disk.Annotations).To(HaveKey(annotations.AnnVDSerial))
		})

		By("Check the source is cleaned up", func() {
			Eventually(func(g Gomega) {
				err := src.GenericClient().Get(ctx, client.ObjectKeyFromObject(virtualMachine), &v1alpha2.VirtualMachine{})
				g.Expect(k8serrors.IsNotFound(err)).To(BeTrue(), "the VM is deleted in the source")
				err = src.GenericClient().Get(ctx, client.ObjectKeyFromObject(virtualDisk), &v1alpha2.VirtualDisk{})
				g.Expect(k8serrors.IsNotFound(err)).To(BeTrue(), "the disk is deleted in the source")
				vmips := &v1alpha2.VirtualMachineIPAddressList{}
				g.Expect(src.GenericClient().List(ctx, vmips, client.InNamespace(src.Namespace().Name))).To(Succeed())
				g.Expect(vmips.Items).To(BeEmpty(), "the address is released in the source")
				err = dst.GenericClient().Get(ctx, client.ObjectKey{Namespace: dst.Namespace().Name, Name: virtualMachine.Name}, &intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver{})
				g.Expect(k8serrors.IsNotFound(err)).To(BeTrue(), "the request is deleted in the peer")
			}).WithTimeout(framework.MiddleTimeout).WithPolling(framework.PollingInterval).Should(Succeed())
		})
	})

	It("admits a request to receive from a peer cluster through the restricted access policy", func() {
		// The peering module presents a peer cluster as peering:peer:<peer>:cap:virtualization, a user without
		// groups; the policy closing the internal API group must let it write the requests to receive and
		// nothing else. The groups here stand for the rights the peering module grants the peer.
		peerUser := "peering:peer:" + peer + ":cap:virtualization"
		asPeer, err := framework.GetClients().ImpersonatedClient(peerUser, "system:masters")
		Expect(err).NotTo(HaveOccurred())
		request := &intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver{
			ObjectMeta: metav1.ObjectMeta{Namespace: dst.Namespace().Name, Name: "policy-check"},
			Spec: intvirtv1alpha1.VirtualMachineReceiverSpec{
				VirtualMachineTemplate:                       runtime.RawExtension{Raw: []byte(`{"metadata":{"name":"policy-check"},"spec":{}}`)},
				InternalVirtualizationVirtualMachineInstance: runtime.RawExtension{Raw: []byte(`{"spec":{}}`)},
				IPAddress: "10.66.10.1",
			},
		}
		Expect(asPeer.Create(ctx, request, client.DryRunAll)).To(Succeed(), "the peer may create a request to receive")

		// An administrator cancels an incoming move by deleting the request; the policy leaves that to RBAC.
		Expect(asPeer.Create(ctx, request)).To(Succeed())
		dst.DeferDelete(request)
		Expect(dst.GenericClient().Delete(ctx, request)).To(Succeed(), "an administrator may delete a request to receive")

		kvvm := &unstructured.Unstructured{}
		kvvm.SetGroupVersionKind(schema.GroupVersionKind{Group: "internal.virtualization.deckhouse.io", Version: "v1", Kind: "InternalVirtualizationVirtualMachine"})
		kvvm.SetNamespace(dst.Namespace().Name)
		kvvm.SetName("policy-check")
		err = asPeer.Create(ctx, kvvm, client.DryRunAll)
		Expect(err).To(HaveOccurred(), "the peer may not write other internal resources")
		Expect(err.Error()).To(ContainSubstring("virtualization-restricted-access-policy"))
	})

	It("keeps the VM in the source when the peer rejects it", func() {
		foreign := object.NewBlankVD(virtualDisk.Name, dst.Namespace().Name, nil, ptr.To(resource.MustParse(vdCustomImageSize)))

		By("Take the name of the disk in the peer", func() {
			Expect(dst.CreateWithDeferredDeletion(ctx, foreign)).To(Succeed())
		})

		By("Run a VM in the source namespace", runVM)

		By("Move the VM to the peer", func() {
			vmop := move()
			src.ExpectFailure(vmop)
			Eventually(func(g Gomega) {
				g.Expect(src.GenericClient().Get(ctx, client.ObjectKeyFromObject(vmop), vmop)).To(Succeed())
				g.Expect(vmop.Status.Phase).To(Equal(v1alpha2.VMOPPhaseFailed))
				completed := meta.FindStatusCondition(vmop.Status.Conditions, string(vmopcondition.TypeCompleted))
				g.Expect(completed).NotTo(BeNil())
				g.Expect(completed.Reason).To(Equal(string(vmopcondition.ReasonPeerRejected)))
				g.Expect(completed.Message).To(ContainSubstring(virtualDisk.Name))
			}).WithTimeout(framework.LongTimeout).WithPolling(framework.PollingInterval).Should(Succeed())
		})

		By("Check nothing moved", func() {
			Eventually(func(g Gomega) {
				err := dst.GenericClient().Get(ctx, client.ObjectKey{Namespace: dst.Namespace().Name, Name: virtualMachine.Name}, &intvirtv1alpha1.InternalVirtualizationVirtualMachineReceiver{})
				g.Expect(k8serrors.IsNotFound(err)).To(BeTrue(), "the request is deleted in the peer")
			}).WithTimeout(framework.MiddleTimeout).WithPolling(framework.PollingInterval).Should(Succeed())

			err := dst.GenericClient().Get(ctx, client.ObjectKey{Namespace: dst.Namespace().Name, Name: virtualMachine.Name}, &v1alpha2.VirtualMachine{})
			Expect(k8serrors.IsNotFound(err)).To(BeTrue(), "no VM is created in the peer")
			disk := &v1alpha2.VirtualDisk{}
			Expect(dst.GenericClient().Get(ctx, client.ObjectKeyFromObject(foreign), disk)).To(Succeed(), "the disk of the peer is kept")
			Expect(disk.UID).To(Equal(foreign.UID))

			kept := &v1alpha2.VirtualMachine{}
			Expect(src.GenericClient().Get(ctx, client.ObjectKeyFromObject(virtualMachine), kept)).To(Succeed())
			Expect(kept.Status.Phase).To(Equal(v1alpha2.MachineRunning))
			eventually.SSHReadyAsRoot(src, kept, framework.MiddleTimeout)
		})
	})
})

const (
	peeringMarkerFile = "/root/peering-marker"

	// renewUntilAddressChanges skips the moment of a renewal when eth0 has no address.
	renewUntilAddressChanges = `old=$(ip -4 addr show dev eth0 | grep inet)
while :; do
	cur=$(ip -4 addr show dev eth0 | grep inet)
	[ -n "$cur" ] && [ "$cur" != "$old" ] && break
	kill -USR1 $(cat /var/run/udhcpc.eth0.pid)
	sleep 5
done
`
)

// guestBootIDAndSerial reads the serial from the SCSI page 0x80 past its 4-byte header: the custom image has
// no udev for lsblk to take it from.
func guestBootIDAndSerial(f *framework.Framework, vm *v1alpha2.VirtualMachine) (string, string) {
	GinkgoHelper()
	out, err := f.SSHCommand(vm.Name, vm.Namespace, "cat /proc/sys/kernel/random/boot_id && tail -c +5 /sys/block/sda/device/vpd_pg80",
		framework.WithSSHUser("root"))
	Expect(err).NotTo(HaveOccurred())
	fields := strings.Fields(out)
	Expect(fields).To(HaveLen(2), "boot id and serial of the root disk: %q", out)
	return fields[0], fields[1]
}

// newSelfPeerSecret is the dev Secret of a peer that is this cluster: virtualization-controller reaches its
// own apiserver with its own token, which the webhook of the request accepts.
func newSelfPeerSecret(ctx context.Context, f *framework.Framework, peer, from, to string) (*corev1.Secret, error) {
	token, err := f.KubeClient().CoreV1().ServiceAccounts(framework.ModuleNamespace).CreateToken(ctx, "virtualization-controller",
		&authnv1.TokenRequest{Spec: authnv1.TokenRequestSpec{ExpirationSeconds: ptr.To[int64](3600)}}, metav1.CreateOptions{})
	if err != nil {
		return nil, fmt.Errorf("create a token of virtualization-controller: %w", err)
	}
	rootCA, err := f.KubeClient().CoreV1().ConfigMaps(framework.ModuleNamespace).Get(ctx, "kube-root-ca.crt", metav1.GetOptions{})
	if err != nil {
		return nil, err
	}
	kubeconfig := fmt.Sprintf(`apiVersion: v1
kind: Config
clusters: [{name: self, cluster: {server: "https://kubernetes.default.svc", certificate-authority-data: %q}}]
users: [{name: self, user: {token: %q}}]
contexts: [{name: self, context: {cluster: self, user: self}}]
current-context: self
`, base64.StdEncoding.EncodeToString([]byte(rootCA.Data["ca.crt"])), token.Status.Token)
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "peering-dev-peer-" + peer, Namespace: framework.ModuleNamespace},
		Data: map[string][]byte{
			"kubeconfig": []byte(kubeconfig),
			"namespaces": []byte(from + "=" + to),
			"newAddress": []byte("true"),
		},
	}, nil
}
