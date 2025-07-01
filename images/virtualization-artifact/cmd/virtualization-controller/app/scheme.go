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

package app

import (
	"fmt"

	vsv1 "github.com/kubernetes-csi/external-snapshotter/client/v6/apis/volumesnapshot/v1"
	extv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	virtv1 "kubevirt.io/api/core/v1"
	cdiv1beta1 "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"

	mcapi "github.com/deckhouse/virtualization-controller/pkg/controller/moduleconfig/api"
	virtv2alpha1 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

func newScheme() (*runtime.Scheme, error) {
	scheme := runtime.NewScheme()
	for _, f := range []func(*runtime.Scheme) error{
		clientgoscheme.AddToScheme,
		extv1.AddToScheme,
		virtv2alpha1.AddToScheme,
		cdiv1beta1.AddToScheme,
		virtv1.AddToScheme,
		vsv1.AddToScheme,
		mcapi.AddToScheme,
	} {
		err := f(scheme)
		if err != nil {
			return nil, fmt.Errorf("failed to add to scheme: %w", err)
		}
	}
	return scheme, nil
}
