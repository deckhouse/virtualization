package storage

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/internalversion"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	genericreq "k8s.io/apiserver/pkg/endpoints/request"
	"k8s.io/apiserver/pkg/registry/rest"
	"k8s.io/client-go/tools/cache"

	virtv2 "github.com/deckhouse/virtualization-controller/api/core/v1alpha2"
)

type VirtualMachineStorage struct {
	groupResource schema.GroupResource
	vmLister      cache.GenericLister
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

func NewStorage(groupResource schema.GroupResource, vmLister cache.GenericLister) *VirtualMachineStorage {
	return &VirtualMachineStorage{
		groupResource: groupResource,
		vmLister:      vmLister,
	}
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

func nameFor(fs fields.Selector) (string, error) {
	if fs == nil {
		fs = fields.Everything()
	}
	name := ""
	if value, found := fs.RequiresExactMatch("metadata.name"); found {
		name = value
	} else if !fs.Empty() {
		return "", fmt.Errorf("field label not supported: %s", fs.Requirements()[0].Field)
	}
	return name, nil
}

func matches(pm *virtv2.VirtualMachine, name string) bool {
	if name == "" {
		name = pm.GetName()
	}
	return pm.GetName() == name
}
