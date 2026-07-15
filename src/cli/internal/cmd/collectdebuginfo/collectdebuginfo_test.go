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

package collectdebuginfo

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"io"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	k8sfake "k8s.io/client-go/kubernetes/fake"
	k8stesting "k8s.io/client-go/testing"

	virtualizationfake "github.com/deckhouse/virtualization/api/client/generated/clientset/versioned/fake"
	virtualizationv1alpha2 "github.com/deckhouse/virtualization/api/client/generated/clientset/versioned/typed/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/client/kubeclient"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

func TestCollectDebugInfo(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "CollectDebugInfo Command Suite")
}

const testNamespace = "default"

type fakeClient struct {
	*k8sfake.Clientset
	virtualizationv1alpha2.VirtualizationV1alpha2Interface
}

func newFakeClient(objects ...runtime.Object) kubeclient.Client {
	return &fakeClient{
		Clientset:                       k8sfake.NewSimpleClientset(),
		VirtualizationV1alpha2Interface: virtualizationfake.NewSimpleClientset(objects...).VirtualizationV1alpha2(),
	}
}

// newFakeClientFailingVMGet returns a client whose VirtualMachine Get fails with
// the given error, used to exercise the target-VM error branches.
func newFakeClientFailingVMGet(err error) kubeclient.Client {
	virt := virtualizationfake.NewSimpleClientset()
	virt.PrependReactor("get", "virtualmachines", func(k8stesting.Action) (bool, runtime.Object, error) {
		return true, nil, err
	})
	return &fakeClient{
		Clientset:                       k8sfake.NewSimpleClientset(),
		VirtualizationV1alpha2Interface: virt.VirtualizationV1alpha2(),
	}
}

func forbidden(resource, name string) error {
	return apierrors.NewForbidden(schema.GroupResource{Group: "virtualization.deckhouse.io", Resource: resource}, name, errors.New("forbidden"))
}

// newDynamicFake returns a dynamic client that knows about the internal
// virtualization resources but holds no objects, so Get returns NotFound and
// List returns an empty list — matching a VM that has no KubeVirt VMI yet.
func newDynamicFake(objects ...runtime.Object) *dynamicfake.FakeDynamicClient {
	const group = "internal.virtualization.deckhouse.io"
	gvrToListKind := map[schema.GroupVersionResource]string{
		{Group: group, Version: "v1", Resource: "internalvirtualizationvirtualmachines"}:                  "InternalVirtualizationVirtualMachineList",
		{Group: group, Version: "v1", Resource: "internalvirtualizationvirtualmachineinstances"}:          "InternalVirtualizationVirtualMachineInstanceList",
		{Group: group, Version: "v1", Resource: "internalvirtualizationvirtualmachineinstancemigrations"}: "InternalVirtualizationVirtualMachineInstanceMigrationList",
	}
	return dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), gvrToListKind, objects...)
}

func newInternalResource(kind, name string) *unstructured.Unstructured {
	obj := &unstructured.Unstructured{}
	obj.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "internal.virtualization.deckhouse.io",
		Version: "v1",
		Kind:    kind,
	})
	obj.SetNamespace(testNamespace)
	obj.SetName(name)
	return obj
}

func newVM(name string, refs ...v1alpha2.BlockDeviceSpecRef) *v1alpha2.VirtualMachine {
	return &v1alpha2.VirtualMachine{
		TypeMeta: metav1.TypeMeta{
			Kind:       "VirtualMachine",
			APIVersion: "virtualization.deckhouse.io/v1alpha2",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: testNamespace,
			UID:       types.UID("uid-" + name),
		},
		Spec: v1alpha2.VirtualMachineSpec{
			BlockDeviceRefs: refs,
		},
	}
}

// runCollect drives collectResources with the given clients and returns the
// files written into the tar.gz archive (keyed by name), the captured stderr,
// and the returned error.
func runCollect(client kubeclient.Client, dyn dynamic.Interface, vmName string) (map[string]string, string, error) {
	var out, errBuf bytes.Buffer
	b := &DebugBundle{
		dynamicClient: dyn,
		stdout:        &out,
		stderr:        &errBuf,
	}
	b.gzipWriter = gzip.NewWriter(&out)
	b.tarWriter = tar.NewWriter(b.gzipWriter)

	err := b.collectResources(context.Background(), client, testNamespace, vmName)

	_ = b.tarWriter.Close()
	_ = b.gzipWriter.Close()

	return readArchive(&out), errBuf.String(), err
}

func readArchive(buf *bytes.Buffer) map[string]string {
	files := map[string]string{}
	gz, err := gzip.NewReader(buf)
	if err != nil {
		return files
	}
	defer func() { _ = gz.Close() }()

	tr := tar.NewReader(gz)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			break
		}
		content, _ := io.ReadAll(tr)
		files[hdr.Name] = string(content)
	}
	return files
}

var _ = Describe("collect-debug-info", func() {
	It("collects a stopped VM even though it has no running VMI", func() {
		client := newFakeClient(newVM("vm-stopped"))

		files, stderr, err := runCollect(client, newDynamicFake(), "vm-stopped")

		Expect(err).NotTo(HaveOccurred())
		Expect(files).To(HaveKey("virtualmachine-vm-stopped.yaml"))
		// Absent VMI: skipped (no error) but surfaced as a warning, not silenced.
		Expect(stderr).To(ContainSubstring(`Warning: InternalVirtualizationVirtualMachineInstance "vm-stopped" not found.`))
	})

	It("skips a disk referenced in the spec that has not been created yet", func() {
		client := newFakeClient(newVM("vm-pending", v1alpha2.BlockDeviceSpecRef{
			Kind: v1alpha2.DiskDevice,
			Name: "missing-disk",
		}))

		files, stderr, err := runCollect(client, newDynamicFake(), "vm-pending")

		Expect(err).NotTo(HaveOccurred())
		Expect(files).To(HaveKey("virtualmachine-vm-pending.yaml"))
		Expect(files).NotTo(HaveKey("virtualdisk-missing-disk.yaml"))
		Expect(stderr).To(ContainSubstring(`Warning: VirtualDisk "missing-disk" not found.`))
	})

	It("collects a disk referenced in the spec that exists", func() {
		vm := newVM("vm-run", v1alpha2.BlockDeviceSpecRef{
			Kind: v1alpha2.DiskDevice,
			Name: "data-disk",
		})
		vd := &v1alpha2.VirtualDisk{
			TypeMeta: metav1.TypeMeta{
				Kind:       "VirtualDisk",
				APIVersion: "virtualization.deckhouse.io/v1alpha2",
			},
			ObjectMeta: metav1.ObjectMeta{Name: "data-disk", Namespace: testNamespace},
		}
		client := newFakeClient(vm, vd)

		files, _, err := runCollect(client, newDynamicFake(), "vm-run")

		Expect(err).NotTo(HaveOccurred())
		Expect(files).To(HaveKey("virtualdisk-data-disk.yaml"))
	})

	It("returns a clear error when the target VM does not exist", func() {
		client := newFakeClient()

		_, _, err := runCollect(client, newDynamicFake(), "ghost")

		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring(`VirtualMachine "ghost" not found in namespace "default"`))
	})

	It("returns an RBAC hint when reading the target VM is forbidden", func() {
		client := newFakeClientFailingVMGet(forbidden("virtualmachines", "vm-x"))

		_, _, err := runCollect(client, newDynamicFake(), "vm-x")

		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("check your RBAC permissions"))
	})

	It("aborts with a wrapped error when reading the target VM fails unexpectedly", func() {
		client := newFakeClientFailingVMGet(errors.New("connection refused"))

		_, _, err := runCollect(client, newDynamicFake(), "vm-x")

		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring(`failed to read VirtualMachine "vm-x"`))
		Expect(err.Error()).To(ContainSubstring("connection refused"))
	})

	It("skips a secondary resource the user is not allowed to read and keeps going", func() {
		dyn := newDynamicFake()
		dyn.PrependReactor("get", "internalvirtualizationvirtualmachines", func(k8stesting.Action) (bool, runtime.Object, error) {
			return true, nil, forbidden("internalvirtualizationvirtualmachines", "vm-x")
		})
		client := newFakeClient(newVM("vm-x"))

		files, stderr, err := runCollect(client, dyn, "vm-x")

		Expect(err).NotTo(HaveOccurred())
		Expect(files).To(HaveKey("virtualmachine-vm-x.yaml"))
		Expect(stderr).To(ContainSubstring(`Warning: cannot read InternalVirtualizationVirtualMachine "vm-x": access denied (check your RBAC permissions).`))
	})

	It("aborts collection on an unexpected error from a secondary resource", func() {
		dyn := newDynamicFake()
		dyn.PrependReactor("get", "internalvirtualizationvirtualmachines", func(k8stesting.Action) (bool, runtime.Object, error) {
			return true, nil, errors.New("etcdserver: request timed out")
		})
		client := newFakeClient(newVM("vm-x"))

		_, _, err := runCollect(client, dyn, "vm-x")

		Expect(err).To(HaveOccurred())
		Expect(err.Error()).To(ContainSubstring("failed to collect VirtualMachine resources"))
	})

	It("collects a disk attached through a VirtualMachineBlockDeviceAttachment", func() {
		vm := newVM("vm-x")
		vmbda := &v1alpha2.VirtualMachineBlockDeviceAttachment{
			TypeMeta: metav1.TypeMeta{
				Kind:       "VirtualMachineBlockDeviceAttachment",
				APIVersion: "virtualization.deckhouse.io/v1alpha2",
			},
			ObjectMeta: metav1.ObjectMeta{Name: "attach-1", Namespace: testNamespace},
			Spec: v1alpha2.VirtualMachineBlockDeviceAttachmentSpec{
				VirtualMachineName: "vm-x",
				BlockDeviceRef: v1alpha2.VMBDAObjectRef{
					Kind: v1alpha2.VMBDAObjectRefKindVirtualDisk,
					Name: "hot-disk",
				},
			},
		}
		vd := &v1alpha2.VirtualDisk{
			TypeMeta:   metav1.TypeMeta{Kind: "VirtualDisk", APIVersion: "virtualization.deckhouse.io/v1alpha2"},
			ObjectMeta: metav1.ObjectMeta{Name: "hot-disk", Namespace: testNamespace},
		}
		client := newFakeClient(vm, vmbda, vd)

		files, _, err := runCollect(client, newDynamicFake(), "vm-x")

		Expect(err).NotTo(HaveOccurred())
		Expect(files).To(HaveKey("virtualmachineblockdeviceattachment-attach-1.yaml"))
		Expect(files).To(HaveKey("virtualdisk-hot-disk.yaml"))
	})

	It("writes an existing internal resource into the bundle", func() {
		client := newFakeClient(newVM("vm-run"))
		dyn := newDynamicFake(newInternalResource("InternalVirtualizationVirtualMachineInstance", "vm-run"))

		files, _, err := runCollect(client, dyn, "vm-run")

		Expect(err).NotTo(HaveOccurred())
		Expect(files).To(HaveKey("internalvirtualizationvirtualmachineinstance-vm-run.yaml"))
	})
})
