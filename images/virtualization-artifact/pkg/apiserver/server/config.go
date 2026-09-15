/*
Copyright 2024 Flant JSC

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

package server

import (
	"errors"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	genericapiserver "k8s.io/apiserver/pkg/server"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/record"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	storagev1alpha1 "github.com/deckhouse/state-snapshotter/api/storage/v1alpha1"
	"github.com/deckhouse/virtualization-controller/pkg/apiserver/api"
	vmrest "github.com/deckhouse/virtualization-controller/pkg/apiserver/registry/vm/rest"
	"github.com/deckhouse/virtualization-controller/pkg/tls/certmanager/filesystem"
	"github.com/deckhouse/virtualization-controller/pkg/unifiedsnapshotter/content"
	"github.com/deckhouse/virtualization-controller/pkg/unifiedsnapshotter/nodeapi"
	"github.com/deckhouse/virtualization-controller/pkg/unifiedsnapshotter/restore"
	"github.com/deckhouse/virtualization/api/client/kubeclient"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

var ErrConfigInvalid = errors.New("configuration is invalid")

type Config struct {
	Apiserver           *genericapiserver.Config
	Rest                *rest.Config
	Kubevirt            vmrest.KubevirtAPIServerConfig
	ProxyClientCertFile string
	ProxyClientKeyFile  string
}

func (c Config) Validate() error {
	var err error
	if c.Kubevirt.Endpoint == "" {
		err = errors.Join(err, fmt.Errorf(".Kubevirt.Endpoint is required. %w", ErrConfigInvalid))
	}
	if c.Kubevirt.CaBundlePath == "" {
		err = errors.Join(err, fmt.Errorf(".Kubevirt.CaBundlePath is required. %w", ErrConfigInvalid))
	}
	if c.Kubevirt.ServiceAccount.Name == "" {
		err = errors.Join(err, fmt.Errorf(".Kubevirt.ServiceAccount.Name is required. %w", ErrConfigInvalid))
	}
	if c.Kubevirt.ServiceAccount.Namespace == "" {
		err = errors.Join(err, fmt.Errorf(".Kubevirt.ServiceAccount.Namespace is required. %w", ErrConfigInvalid))
	}
	if c.ProxyClientCertFile == "" {
		err = errors.Join(err, fmt.Errorf(".ProxyClientCertFile is required. %w", ErrConfigInvalid))
	}
	if c.ProxyClientKeyFile == "" {
		err = errors.Join(err, fmt.Errorf(".ProxyClientKeyFile is required. %w", ErrConfigInvalid))
	}
	if c.Apiserver == nil {
		err = errors.Join(err, fmt.Errorf(".Apiserver is required. %w", ErrConfigInvalid))
	}
	if c.Rest == nil {
		err = errors.Join(err, fmt.Errorf(".Rest is required. %w", ErrConfigInvalid))
	}
	return err
}

func (c Config) Complete() (*Server, error) {
	proxyCertManager := filesystem.NewFileCertificateManager(c.ProxyClientCertFile, c.ProxyClientKeyFile)
	vmSharedInformerFactory, err := virtualizationInformerFactory(c.Rest)
	if err != nil {
		return nil, err
	}
	vmInformer := vmSharedInformerFactory.Virtualization().V1alpha2().VirtualMachines()

	genericServer, err := c.Apiserver.Complete(nil).New("virtualziation-api", genericapiserver.NewEmptyDelegate())
	if err != nil {
		return nil, err
	}

	// Write-capable client used by enterprise subresources (e.g. scaleDownWith) to delete pool
	// members and adjust spec.replicas from the apiserver's own identity, and to hold the console
	// and VNC session leases: it speaks both APIs, the virtualization one and Kubernetes itself.
	virtCli, err := kubeclient.GetClientFromRESTConfig(c.Rest)
	if err != nil {
		return nil, err
	}

	// Reports a session takeover on the virtual machine.
	recorder, err := newEventRecorder(virtCli)
	if err != nil {
		return nil, err
	}

	snapshots, err := newSnapshotServices(c.Rest)
	if err != nil {
		return nil, err
	}

	err = api.Install(vmInformer.Lister(),
		genericServer,
		c.Kubevirt,
		proxyCertManager,
		virtCli,
		recorder,
		snapshots.compiler,
		snapshots.nodes,
	)
	if err != nil {
		return nil, err
	}

	return NewServer(
		vmInformer.Informer(),
		genericServer,
		proxyCertManager,
	), nil
}

// snapshotServices holds what the three per-node snapshot subresources run on.
type snapshotServices struct {
	// compiler backs manifests-with-data-restoration.
	compiler *restore.Compiler
	// nodes backs manifests-download and manifests-and-children-refs-upload.
	nodes *nodeapi.Service
}

// newSnapshotServices builds them over a single client and a single transport to the core.
//
// The client is deliberately uncached: every one of these endpoints decides from a snapshot's status
// whether to read captured manifests, or to write into them, and a cached read could answer from before
// the binder or before a mode was settled. It also carries the state-snapshotter scheme, because none of
// the three trusts a status.boundSnapshotContentName on its own — the SnapshotContent it names has to
// point back at the snapshot that named it, and that check needs to read the content itself.
func newSnapshotServices(cfg *rest.Config) (*snapshotServices, error) {
	scheme := runtime.NewScheme()
	if err := v1alpha2.AddToScheme(scheme); err != nil {
		return nil, err
	}
	if err := storagev1alpha1.AddToScheme(scheme); err != nil {
		return nil, err
	}

	cli, err := ctrlclient.New(cfg, ctrlclient.Options{Scheme: scheme})
	if err != nil {
		return nil, fmt.Errorf("build snapshot client: %w", err)
	}

	core, err := content.NewClient(cfg)
	if err != nil {
		return nil, err
	}

	return &snapshotServices{
		compiler: restore.NewCompiler(cli, restore.NewContentFetcher(core)),
		nodes:    nodeapi.NewService(cli, cli, core),
	}, nil
}

// newEventRecorder builds a recorder that reports events on virtual machines. The broadcaster
// lives as long as the process: the apiserver has no shutdown hook to attach it to.
func newEventRecorder(kubeCli kubeclient.Client) (record.EventRecorder, error) {
	scheme := runtime.NewScheme()
	if err := v1alpha2.AddToScheme(scheme); err != nil {
		return nil, err
	}
	broadcaster := record.NewBroadcaster()
	broadcaster.StartRecordingToSink(&typedcorev1.EventSinkImpl{Interface: kubeCli.CoreV1().Events("")})
	return broadcaster.NewRecorder(scheme, corev1.EventSource{Component: "virtualization-api"}), nil
}
