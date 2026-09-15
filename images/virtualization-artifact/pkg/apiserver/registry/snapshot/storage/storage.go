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

package storage

import (
	"context"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	genericreq "k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/apiserver/pkg/registry/rest"

	snapshotrest "github.com/deckhouse/virtualization-controller/pkg/apiserver/registry/snapshot/rest"
	"github.com/deckhouse/virtualization-controller/pkg/unifiedsnapshotter/nodeapi"
	"github.com/deckhouse/virtualization-controller/pkg/unifiedsnapshotter/restore"
	"github.com/deckhouse/virtualization/api/client/kubeclient"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/subresources"
	subv1alpha2 "github.com/deckhouse/virtualization/api/subresources/v1alpha2"
)

type VirtualMachineSnapshotStorage struct {
	virtCli   kubeclient.Client
	manifests *snapshotrest.ManifestsWithDataRestorationREST
	download  *snapshotrest.ManifestsDownloadREST
	upload    *snapshotrest.ManifestsAndChildrenRefsUploadREST
}

var (
	_ rest.KindProvider         = &VirtualMachineSnapshotStorage{}
	_ rest.Storage              = &VirtualMachineSnapshotStorage{}
	_ rest.Scoper               = &VirtualMachineSnapshotStorage{}
	_ rest.Getter               = &VirtualMachineSnapshotStorage{}
	_ rest.SingularNameProvider = &VirtualMachineSnapshotStorage{}
)

func NewVirtualMachineSnapshotStorage(virtCli kubeclient.Client, compiler *restore.Compiler, nodes *nodeapi.Service) *VirtualMachineSnapshotStorage {
	return &VirtualMachineSnapshotStorage{
		virtCli:   virtCli,
		manifests: snapshotrest.NewManifestsWithDataRestorationREST(v1alpha2.VirtualMachineSnapshotResource, compiler),
		download:  snapshotrest.NewManifestsDownloadREST(v1alpha2.VirtualMachineSnapshotResource, nodes),
		upload:    snapshotrest.NewManifestsAndChildrenRefsUploadREST(v1alpha2.VirtualMachineSnapshotResource, nodes),
	}
}

func (store VirtualMachineSnapshotStorage) ManifestsWithDataRestorationREST() *snapshotrest.ManifestsWithDataRestorationREST {
	return store.manifests
}

func (store VirtualMachineSnapshotStorage) ManifestsDownloadREST() *snapshotrest.ManifestsDownloadREST {
	return store.download
}

func (store VirtualMachineSnapshotStorage) ManifestsAndChildrenRefsUploadREST() *snapshotrest.ManifestsAndChildrenRefsUploadREST {
	return store.upload
}

func (store VirtualMachineSnapshotStorage) New() runtime.Object {
	return &subv1alpha2.VirtualMachineSnapshot{}
}

func (store VirtualMachineSnapshotStorage) Destroy() {}

func (store VirtualMachineSnapshotStorage) Kind() string {
	return v1alpha2.VirtualMachineSnapshotKind
}

func (store VirtualMachineSnapshotStorage) NamespaceScoped() bool {
	return true
}

func (store VirtualMachineSnapshotStorage) GetSingularName() string {
	return "virtualmachinesnapshot"
}

func (store VirtualMachineSnapshotStorage) Get(ctx context.Context, name string, opts *metav1.GetOptions) (runtime.Object, error) {
	namespace := genericreq.NamespaceValue(ctx)
	getOpts := metav1.GetOptions{}
	if opts != nil {
		getOpts = *opts
	}
	vms, err := store.virtCli.VirtualMachineSnapshots(namespace).Get(ctx, name, getOpts)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return nil, k8serrors.NewNotFound(subresources.Resource(v1alpha2.VirtualMachineSnapshotResource), name)
		}
		return nil, k8serrors.NewInternalError(err)
	}
	return &subresources.VirtualMachineSnapshot{
		TypeMeta: metav1.TypeMeta{
			APIVersion: subresources.SchemeGroupVersion.String(),
			Kind:       v1alpha2.VirtualMachineSnapshotKind,
		},
		ObjectMeta: vms.ObjectMeta,
	}, nil
}

type VirtualDiskSnapshotStorage struct {
	virtCli   kubeclient.Client
	manifests *snapshotrest.ManifestsWithDataRestorationREST
	download  *snapshotrest.ManifestsDownloadREST
	upload    *snapshotrest.ManifestsAndChildrenRefsUploadREST
}

var (
	_ rest.KindProvider         = &VirtualDiskSnapshotStorage{}
	_ rest.Storage              = &VirtualDiskSnapshotStorage{}
	_ rest.Scoper               = &VirtualDiskSnapshotStorage{}
	_ rest.Getter               = &VirtualDiskSnapshotStorage{}
	_ rest.SingularNameProvider = &VirtualDiskSnapshotStorage{}
)

func NewVirtualDiskSnapshotStorage(virtCli kubeclient.Client, compiler *restore.Compiler, nodes *nodeapi.Service) *VirtualDiskSnapshotStorage {
	return &VirtualDiskSnapshotStorage{
		virtCli:   virtCli,
		manifests: snapshotrest.NewManifestsWithDataRestorationREST(v1alpha2.VirtualDiskSnapshotResource, compiler),
		download:  snapshotrest.NewManifestsDownloadREST(v1alpha2.VirtualDiskSnapshotResource, nodes),
		upload:    snapshotrest.NewManifestsAndChildrenRefsUploadREST(v1alpha2.VirtualDiskSnapshotResource, nodes),
	}
}

func (store VirtualDiskSnapshotStorage) ManifestsWithDataRestorationREST() *snapshotrest.ManifestsWithDataRestorationREST {
	return store.manifests
}

func (store VirtualDiskSnapshotStorage) ManifestsDownloadREST() *snapshotrest.ManifestsDownloadREST {
	return store.download
}

func (store VirtualDiskSnapshotStorage) ManifestsAndChildrenRefsUploadREST() *snapshotrest.ManifestsAndChildrenRefsUploadREST {
	return store.upload
}

func (store VirtualDiskSnapshotStorage) New() runtime.Object {
	return &subv1alpha2.VirtualDiskSnapshot{}
}

func (store VirtualDiskSnapshotStorage) Destroy() {}

func (store VirtualDiskSnapshotStorage) Kind() string {
	return v1alpha2.VirtualDiskSnapshotKind
}

func (store VirtualDiskSnapshotStorage) NamespaceScoped() bool {
	return true
}

func (store VirtualDiskSnapshotStorage) GetSingularName() string {
	return "virtualdisksnapshot"
}

func (store VirtualDiskSnapshotStorage) Get(ctx context.Context, name string, opts *metav1.GetOptions) (runtime.Object, error) {
	namespace := genericreq.NamespaceValue(ctx)
	getOpts := metav1.GetOptions{}
	if opts != nil {
		getOpts = *opts
	}
	vds, err := store.virtCli.VirtualDiskSnapshots(namespace).Get(ctx, name, getOpts)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return nil, k8serrors.NewNotFound(subresources.Resource(v1alpha2.VirtualDiskSnapshotResource), name)
		}
		return nil, k8serrors.NewInternalError(err)
	}
	return &subresources.VirtualDiskSnapshot{
		TypeMeta: metav1.TypeMeta{
			APIVersion: subresources.SchemeGroupVersion.String(),
			Kind:       v1alpha2.VirtualDiskSnapshotKind,
		},
		ObjectMeta: vds.ObjectMeta,
	}, nil
}
