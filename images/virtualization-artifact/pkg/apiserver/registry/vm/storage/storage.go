package storage

import (
	"context"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/internalversion"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	genericreq "k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/apiserver/pkg/registry/rest"
	"k8s.io/client-go/tools/cache"

	virtv2 "github.com/deckhouse/virtualization-controller/api/core/v1alpha2"
	vmrest "github.com/deckhouse/virtualization-controller/pkg/apiserver/registry/vm/rest"
	"github.com/deckhouse/virtualization-controller/pkg/tls/certManager"
)

type VirtualMachineStorage struct {
	groupResource schema.GroupResource
	vmLister      cache.GenericLister
	console       *vmrest.ConsoleREST
	vnc           *vmrest.VNCREST
	portforward   *vmrest.PortForwardREST
	rest.TableConvertor
}

var (
	_ rest.KindProvider         = &VirtualMachineStorage{}
	_ rest.Storage              = &VirtualMachineStorage{}
	_ rest.Scoper               = &VirtualMachineStorage{}
	_ rest.Lister               = &VirtualMachineStorage{}
	_ rest.Getter               = &VirtualMachineStorage{}
	_ rest.TableConvertor       = &VirtualMachineStorage{}
	_ rest.SingularNameProvider = &VirtualMachineStorage{}
)

func NewStorage(
	groupResource schema.GroupResource,
	vmLister cache.GenericLister,
	kubevirt vmrest.KubevirtApiServerConfig,
	proxyCertManager certManager.CertificateManager,
) *VirtualMachineStorage {
	return &VirtualMachineStorage{
		groupResource: groupResource,
		vmLister:      vmLister,
		console:       vmrest.NewConsoleREST(vmLister, kubevirt, proxyCertManager),
		vnc:           vmrest.NewVNCREST(vmLister, kubevirt, proxyCertManager),
		portforward:   vmrest.NewPortForwardREST(vmLister, kubevirt, proxyCertManager),
	}
}

func (store VirtualMachineStorage) ConsoleREST() *vmrest.ConsoleREST {
	return store.console
}

func (store VirtualMachineStorage) VncREST() *vmrest.VNCREST {
	return store.vnc
}

func (store VirtualMachineStorage) PortForwardREST() *vmrest.PortForwardREST {
	return store.portforward
}

// New implements rest.Storage interface
func (store VirtualMachineStorage) New() runtime.Object {
	return &virtv2.VirtualMachine{}
}

// Destroy implements rest.Storage interface
func (store VirtualMachineStorage) Destroy() {
}

// Kind implements rest.KindProvider interface
func (store VirtualMachineStorage) Kind() string {
	return "VirtualMachine"
}

// NamespaceScoped implements rest.Scoper interface
func (store VirtualMachineStorage) NamespaceScoped() bool {
	return true
}

// GetSingularName implements rest.SingularNameProvider interface
func (store VirtualMachineStorage) GetSingularName() string {
	return "virtualmachine"
}

func (store VirtualMachineStorage) Get(ctx context.Context, name string, _ *metav1.GetOptions) (runtime.Object, error) {
	namespace := genericreq.NamespaceValue(ctx)
	vm, err := store.vmLister.ByNamespace(namespace).Get(name)
	if err != nil || vm == nil {
		return nil, apierrors.NewNotFound(store.groupResource, name)
	}
	return vm, nil
}

func (store VirtualMachineStorage) NewList() runtime.Object {
	return &virtv2.VirtualMachineList{}
}

func (store VirtualMachineStorage) List(ctx context.Context, options *internalversion.ListOptions) (runtime.Object, error) {
	namespace := genericreq.NamespaceValue(ctx)

	labelSelector := labels.Everything()
	if options != nil && options.LabelSelector != nil {
		labelSelector = options.LabelSelector
	}

	name, err := nameFor(options.FieldSelector)
	if err != nil {
		return nil, err
	}

	items, err := store.vmLister.ByNamespace(namespace).List(labelSelector)
	if err != nil {
		return nil, apierrors.NewInternalError(err)
	}

	filtered := &virtv2.VirtualMachineList{}
	filtered.Items = make([]virtv2.VirtualMachine, 0, len(items))
	for _, manifest := range items {
		vm, ok := manifest.(*virtv2.VirtualMachine)
		if !ok || vm == nil {
			continue
		}
		if matches(vm, name) {
			filtered.Items = append(filtered.Items, *vm)
		}
	}
	return filtered, nil
}
