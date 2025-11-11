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

package testutil

import (
	"context"
	"log/slog"
	"reflect"

	"github.com/go-logr/logr"
	apiruntime "k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	virtv1 "kubevirt.io/api/core/v1"
	cdiv1 "kubevirt.io/containerized-data-importer-api/pkg/apis/core/v1beta1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	"github.com/deckhouse/deckhouse/pkg/log"
	"github.com/deckhouse/virtualization-controller/pkg/controller/indexer"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha3"
)

func NewFakeClientWithObjects(objs ...client.Object) (client.WithWatch, error) {
	scheme := apiruntime.NewScheme()
	for _, f := range []func(*apiruntime.Scheme) error{
		v1alpha2.AddToScheme,
		v1alpha3.AddToScheme,
		virtv1.AddToScheme,
		cdiv1.AddToScheme,
		clientgoscheme.AddToScheme,
	} {
		err := f(scheme)
		if err != nil {
			return nil, err
		}
	}
	var newObjs []client.Object
	for _, obj := range objs {
		if reflect.ValueOf(obj).IsNil() {
			continue
		}
		newObjs = append(newObjs, obj)
	}
	b := fake.NewClientBuilder().WithScheme(scheme).WithObjects(newObjs...).WithStatusSubresource(newObjs...)
	for _, fn := range indexer.IndexGetters {
		b.WithIndex(fn())
	}

	return b.Build(), nil
}

func NewFakeClientWithInterceptorWithObjects(interceptor interceptor.Funcs, objs ...client.Object) (client.WithWatch, error) {
	scheme := apiruntime.NewScheme()
	for _, f := range []func(*apiruntime.Scheme) error{
		v1alpha2.AddToScheme,
		v1alpha3.AddToScheme,
		virtv1.AddToScheme,
		cdiv1.AddToScheme,
		clientgoscheme.AddToScheme,
	} {
		err := f(scheme)
		if err != nil {
			return nil, err
		}
	}
	var newObjs []client.Object
	for _, obj := range objs {
		if reflect.ValueOf(obj).IsNil() {
			continue
		}
		newObjs = append(newObjs, obj)
	}
	b := fake.NewClientBuilder().WithScheme(scheme).WithObjects(newObjs...).WithStatusSubresource(newObjs...).WithInterceptorFuncs(interceptor)
	for _, fn := range indexer.IndexGetters {
		b.WithIndex(fn())
	}

	return b.Build(), nil
}

func NewNoOpLogger() *log.Logger {
	return log.NewNop()
}

func NewNoOpSlogLogger() *slog.Logger {
	return slog.New(log.NewNop().Handler())
}

func ToContext(ctx context.Context, log *log.Logger) context.Context {
	return logr.NewContextWithSlogLogger(ctx, slog.New(log.Handler()))
}

func ContextBackgroundWithNoOpLogger() context.Context {
	return ToContext(context.Background(), NewNoOpLogger())
}
