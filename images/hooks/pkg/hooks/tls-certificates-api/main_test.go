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

package tls_certificates_api

import (
	"context"
	"testing"

	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/module-sdk/pkg"
	"github.com/deckhouse/module-sdk/testing/mock"
	mcapi "github.com/deckhouse/virtualization-controller/pkg/controller/moduleconfig/api"
	"github.com/deckhouse/virtualization/hooks/pkg/settings"
)

type fakeKubernetesClient struct {
	pkg.KubernetesClient
	get func(ctx context.Context, key ctrlclient.ObjectKey, obj ctrlclient.Object) error
}

func (f *fakeKubernetesClient) Get(ctx context.Context, key ctrlclient.ObjectKey, obj ctrlclient.Object, _ ...ctrlclient.GetOption) error {
	return f.get(ctx, key, obj)
}

func TestBeforeHookCheckSkipsWithoutModuleConfig(t *testing.T) {
	dc := mock.NewDependencyContainerMock(t)
	dc.GetK8sClientMock.Return(&fakeKubernetesClient{get: func(ctx context.Context, key ctrlclient.ObjectKey, obj ctrlclient.Object) error {
		mc := obj.(*mcapi.ModuleConfig)
		*mc = *settings.NewModuleConfigForTest(nil)
		return nil
	}}, nil)

	if conf.BeforeHookCheck == nil {
		t.Fatal("expected BeforeHookCheck to be configured")
	}

	if ok := conf.BeforeHookCheck(&pkg.HookInput{DC: dc}); ok {
		t.Fatalf("expected BeforeHookCheck to return false")
	}
}
