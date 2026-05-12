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

package settings

import (
	"context"
	"testing"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ctrlclient "sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/module-sdk/pkg"
	patchablevalues "github.com/deckhouse/module-sdk/pkg/patchable-values"
	"github.com/deckhouse/module-sdk/testing/mock"
	mcapi "github.com/deckhouse/virtualization-controller/pkg/controller/moduleconfig/api"
)

type fakeKubernetesClient struct {
	pkg.KubernetesClient
	get func(ctx context.Context, key ctrlclient.ObjectKey, obj ctrlclient.Object) error
}

func (f *fakeKubernetesClient) Get(ctx context.Context, key ctrlclient.ObjectKey, obj ctrlclient.Object, _ ...ctrlclient.GetOption) error {
	return f.get(ctx, key, obj)
}

func TestCanRunWithModuleConfig(t *testing.T) {
	newInput := func(client pkg.KubernetesClient, err error) *pkg.HookInput {
		dc := mock.NewDependencyContainerMock(t)
		dc.GetK8sClientMock.Return(client, err)
		return &pkg.HookInput{DC: dc}
	}

	t.Run("returns true when internal values copy exists", func(t *testing.T) {
		dc := mock.NewDependencyContainerMock(t)
		values, err := patchablevalues.NewPatchableValues(map[string]any{
			"virtualization": map[string]any{
				"internal": map[string]any{
					"moduleConfig": map[string]any{},
				},
			},
		})
		if err != nil {
			t.Fatalf("create patchable values: %v", err)
		}

		input := &pkg.HookInput{DC: dc, Values: values}
		ok, err := CanRunWithModuleConfig(context.Background(), input)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !ok {
			t.Fatalf("expected CanRunWithModuleConfig to return true")
		}
	})

	t.Run("returns false when module config does not exist", func(t *testing.T) {
		input := newInput(&fakeKubernetesClient{get: func(ctx context.Context, key ctrlclient.ObjectKey, obj ctrlclient.Object) error {
			return apierrors.NewNotFound(schema.GroupResource{Group: "deckhouse.io", Resource: "moduleconfigs"}, ModuleName)
		}}, nil)

		ok, err := CanRunWithModuleConfig(context.Background(), input)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ok {
			t.Fatalf("expected CanRunWithModuleConfig to return false")
		}
	})

	t.Run("returns false when settings are nil", func(t *testing.T) {
		input := newInput(&fakeKubernetesClient{get: func(ctx context.Context, key ctrlclient.ObjectKey, obj ctrlclient.Object) error {
			mc := obj.(*mcapi.ModuleConfig)
			*mc = *NewModuleConfigForTest(nil)
			return nil
		}}, nil)

		ok, err := CanRunWithModuleConfig(context.Background(), input)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ok {
			t.Fatalf("expected CanRunWithModuleConfig to return false")
		}
	})

	t.Run("returns false when required settings are absent", func(t *testing.T) {
		input := newInput(&fakeKubernetesClient{get: func(ctx context.Context, key ctrlclient.ObjectKey, obj ctrlclient.Object) error {
			mc := obj.(*mcapi.ModuleConfig)
			*mc = *NewModuleConfigForTest(map[string]any{})
			return nil
		}}, nil)

		ok, err := CanRunWithModuleConfig(context.Background(), input)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if ok {
			t.Fatalf("expected CanRunWithModuleConfig to return false")
		}
	})

	t.Run("returns true when required settings exist", func(t *testing.T) {
		input := newInput(&fakeKubernetesClient{get: func(ctx context.Context, key ctrlclient.ObjectKey, obj ctrlclient.Object) error {
			mc := obj.(*mcapi.ModuleConfig)
			*mc = *NewModuleConfigForTest(map[string]any{
				"virtualMachineCIDRs": []any{"10.0.0.0/24"},
				"dvcr":                map[string]any{},
			})
			return nil
		}}, nil)

		ok, err := CanRunWithModuleConfig(context.Background(), input)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !ok {
			t.Fatalf("expected CanRunWithModuleConfig to return true")
		}
	})

	t.Run("returns error when kubernetes client cannot be created", func(t *testing.T) {
		input := newInput(nil, staticError("boom"))

		ok, err := CanRunWithModuleConfig(context.Background(), input)
		if err == nil {
			t.Fatalf("expected error")
		}
		if ok {
			t.Fatalf("expected CanRunWithModuleConfig to return false")
		}
	})
}

type staticError string

func (e staticError) Error() string { return string(e) }
