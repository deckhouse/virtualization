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
	"fmt"

	ciliumClient "github.com/cilium/cilium/pkg/k8s/client/clientset/versioned"
	ciliumInformers "github.com/cilium/cilium/pkg/k8s/client/informers/externalversions"
	"k8s.io/client-go/rest"

	virtClient "github.com/deckhouse/virtualization/api/client/generated/clientset/versioned"
	virtInformers "github.com/deckhouse/virtualization/api/client/generated/informers/externalversions"
)

const (
	// we should never need to resync, since we're not worried about missing events,
	// and resync is actually for regular interval-based reconciliation these days,
	// so set the default resync interval to 0
	defaultResync = 0
)

func VirtualizationInformerFactory(rest *rest.Config) (virtInformers.SharedInformerFactory, error) {
	client, err := virtClient.NewForConfig(rest)
	if err != nil {
		return nil, fmt.Errorf("unable to construct lister client: %w", err)
	}
	return virtInformers.NewSharedInformerFactory(client, defaultResync), nil
}

func CiliumInformerFactory(restConfig *rest.Config) (ciliumInformers.SharedInformerFactory, error) {
	client, err := ciliumClient.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("unable to create Cilium client: %w", err)
	}
	return ciliumInformers.NewSharedInformerFactory(client, defaultResync), nil
}
