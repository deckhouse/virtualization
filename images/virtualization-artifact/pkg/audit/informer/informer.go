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

package informer

import (
	"context"
	"errors"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/rest"
	kubecache "k8s.io/client-go/tools/cache"

	"github.com/deckhouse/deckhouse/pkg/log"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type cache interface {
	Add(key string, obj any)
}

type InformerList struct {
	vmInformer           kubecache.SharedIndexInformer
	vdInformer           kubecache.SharedIndexInformer
	vmopInformer         kubecache.SharedIndexInformer
	podInformer          kubecache.SharedIndexInformer
	nodeInformer         kubecache.SharedIndexInformer
	moduleInformer       kubecache.SharedIndexInformer
	moduleConfigInformer kubecache.SharedIndexInformer
	internalVMIInformer  kubecache.SharedIndexInformer
}

func NewInformerList(ctx context.Context, kubeCfg *rest.Config, ttlCache cache) (*InformerList, error) {
	inf := &InformerList{}
	virtSharedInformerFactory, err := VirtualizationInformerFactory(kubeCfg)
	if err != nil {
		log.Error("failed to create virtualization shared factory", log.Err(err))
		return inf, err
	}

	coreSharedInformerFactory, err := CoreInformerFactory(kubeCfg)
	if err != nil {
		log.Error("failed to create core shared factory", log.Err(err))
		return inf, err
	}

	dynamicInformerFactory, err := DynamicInformerFactory(kubeCfg)
	if err != nil {
		log.Error("failed to create dynamic informer factory", log.Err(err))
		return inf, err
	}

	vmInformer := virtSharedInformerFactory.Virtualization().V1alpha2().VirtualMachines().Informer()
	_, err = vmInformer.AddEventHandler(kubecache.ResourceEventHandlerFuncs{
		DeleteFunc: func(obj any) {
			_, ok := obj.(kubecache.DeletedFinalStateUnknown)
			if ok {
				return
			}

			vm := obj.(*v1alpha2.VirtualMachine)
			key := fmt.Sprintf("virtualmachines/%s/%s", vm.Namespace, vm.Name)
			ttlCache.Add(key, vm)
		},
	})
	if err != nil {
		log.Error("failed to add event handler for virtual machines", log.Err(err))
		return inf, err
	}
	inf.vmInformer = vmInformer

	vdInformer := virtSharedInformerFactory.Virtualization().V1alpha2().VirtualDisks().Informer()
	_, err = vdInformer.AddEventHandler(kubecache.ResourceEventHandlerFuncs{
		DeleteFunc: func(obj any) {
			_, ok := obj.(kubecache.DeletedFinalStateUnknown)
			if ok {
				return
			}

			vd := obj.(*v1alpha2.VirtualDisk)
			key := fmt.Sprintf("pods/%s/%s", vd.Namespace, vd.Name)
			ttlCache.Add(key, vd)
		},
	})
	if err != nil {
		log.Error("failed to add event handler for virtual disks", log.Err(err))
		return inf, err
	}
	inf.vdInformer = vdInformer

	podInformer := coreSharedInformerFactory.Core().V1().Pods().Informer()
	_, err = podInformer.AddEventHandler(kubecache.ResourceEventHandlerFuncs{
		DeleteFunc: func(obj any) {
			_, ok := obj.(kubecache.DeletedFinalStateUnknown)
			if ok {
				return
			}

			pod := obj.(*corev1.Pod)
			key := fmt.Sprintf("pods/%s/%s", pod.Namespace, pod.Name)
			ttlCache.Add(key, pod)
		},
	})
	if err != nil {
		log.Error("failed to add event handler for pods", log.Err(err))
		return inf, err
	}
	inf.podInformer = podInformer

	internalVMIInformer := GetInternalVMIInformer(dynamicInformerFactory).Informer()
	_, err = internalVMIInformer.AddEventHandler(kubecache.ResourceEventHandlerFuncs{
		DeleteFunc: func(obj any) {
			_, ok := obj.(kubecache.DeletedFinalStateUnknown)
			if ok {
				return
			}

			unstructuredObj, ok := obj.(*unstructured.Unstructured)
			if !ok {
				return
			}
			key := fmt.Sprintf("intVMI/%s/%s", unstructuredObj.GetNamespace(), unstructuredObj.GetName())
			ttlCache.Add(key, unstructuredObj)
		},
	})
	if err != nil {
		log.Error("failed to add event handler for internalVMI", log.Err(err))
		return inf, err
	}
	inf.internalVMIInformer = internalVMIInformer

	vmopInformer := virtSharedInformerFactory.Virtualization().V1alpha2().VirtualMachineOperations().Informer()
	inf.vmopInformer = vmopInformer

	nodeInformer := coreSharedInformerFactory.Core().V1().Nodes().Informer()
	inf.nodeInformer = nodeInformer

	moduleInformer := GetModuleInformer(dynamicInformerFactory).Informer()
	inf.moduleInformer = moduleInformer

	moduleConfigInformer := GetModuleConfigsInformer(dynamicInformerFactory).Informer()
	inf.moduleConfigInformer = moduleConfigInformer

	return inf, nil
}

func (i *InformerList) Run(ctx context.Context) error {
	go i.podInformer.Run(ctx.Done())
	go i.nodeInformer.Run(ctx.Done())
	go i.vmInformer.Run(ctx.Done())
	go i.vdInformer.Run(ctx.Done())
	go i.vmopInformer.Run(ctx.Done())
	go i.moduleInformer.Run(ctx.Done())
	go i.moduleConfigInformer.Run(ctx.Done())
	go i.internalVMIInformer.Run(ctx.Done())

	if !kubecache.WaitForCacheSync(
		ctx.Done(),
		i.podInformer.HasSynced,
		i.nodeInformer.HasSynced,
		i.vmInformer.HasSynced,
		i.vdInformer.HasSynced,
		i.vmopInformer.HasSynced,
		i.moduleInformer.HasSynced,
		i.moduleConfigInformer.HasSynced,
		i.internalVMIInformer.HasSynced,
	) {
		return errors.New("failed to wait for caches to sync")
	}

	return nil
}

func (i *InformerList) GetVMInformer() kubecache.Store {
	return i.vmInformer.GetIndexer()
}

func (i *InformerList) GetVDInformer() kubecache.Store {
	return i.vdInformer.GetIndexer()
}

func (i *InformerList) GetVMOPInformer() kubecache.Store {
	return i.vmopInformer.GetIndexer()
}

func (i *InformerList) GetPodInformer() kubecache.Store {
	return i.podInformer.GetIndexer()
}

func (i *InformerList) GetNodeInformer() kubecache.Store {
	return i.nodeInformer.GetIndexer()
}

func (i *InformerList) GetModuleInformer() kubecache.Store {
	return i.moduleInformer.GetIndexer()
}

func (i *InformerList) GetModuleConfigInformer() kubecache.Store {
	return i.moduleConfigInformer.GetIndexer()
}

func (i *InformerList) GetInternalVMIInformer() kubecache.Store {
	return i.internalVMIInformer.GetIndexer()
}
