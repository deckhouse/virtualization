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

package store

import (
	"context"
	"time"

	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	Namespace     = "d8-virtualization"
	ConfigMapName = "virtualization-controller-migrations"
)

type Store interface {
	IsCompleted(ctx context.Context, name string) (bool, error)
	MarkCompleted(ctx context.Context, name string) error
}

type ConfigMapStore struct {
	client client.Client
}

func NewConfigMapStore(client client.Client) *ConfigMapStore {
	return &ConfigMapStore{client: client}
}

func (s *ConfigMapStore) IsCompleted(ctx context.Context, name string) (bool, error) {
	cm := &corev1.ConfigMap{}
	err := s.client.Get(ctx, key(), cm)
	if k8serrors.IsNotFound(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	_, ok := cm.Data[name]
	return ok, nil
}

func (s *ConfigMapStore) MarkCompleted(ctx context.Context, name string) error {
	if err := s.ensure(ctx); err != nil {
		return err
	}

	completedAt := time.Now().UTC().Format(time.RFC3339)
	return retry.RetryOnConflict(retry.DefaultBackoff, func() error {
		cm := &corev1.ConfigMap{}
		if err := s.client.Get(ctx, key(), cm); err != nil {
			return err
		}

		if cm.Data == nil {
			cm.Data = make(map[string]string, 1)
		}
		if _, ok := cm.Data[name]; ok {
			return nil
		}

		cm.Data[name] = completedAt
		return s.client.Update(ctx, cm)
	})
}

func (s *ConfigMapStore) ensure(ctx context.Context) error {
	cm := &corev1.ConfigMap{}
	err := s.client.Get(ctx, key(), cm)
	if err == nil {
		return nil
	}
	if !k8serrors.IsNotFound(err) {
		return err
	}

	cm = &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ConfigMapName,
			Namespace: Namespace,
		},
		Data: map[string]string{},
	}
	err = s.client.Create(ctx, cm)
	if k8serrors.IsAlreadyExists(err) {
		return nil
	}
	return err
}

func key() types.NamespacedName {
	return types.NamespacedName{Name: ConfigMapName, Namespace: Namespace}
}
