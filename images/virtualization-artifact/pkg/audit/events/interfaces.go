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

package events

import (
	"context"

	"k8s.io/apiserver/pkg/apis/audit"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/cache"
)

//go:generate go tool moq -rm -out mock.go . TTLCache InformerList EventLoggerOptions

type TTLCache interface {
	Get(key string) (any, bool)
}

type EventLogger interface {
	IsMatched() bool
	Fill() error
	ShouldLog() bool
	Log() error
}

type EventLoggerOptions interface {
	GetTTLCache() TTLCache
	GetCtx() context.Context
	GetEvent() *audit.Event
	GetInformerList() InformerList
	GetClient() kubernetes.Interface
}

type InformerList interface {
	GetVMInformer() cache.Store
	GetVDInformer() cache.Store
	GetVMOPInformer() cache.Store
	GetPodInformer() cache.Store
	GetNodeInformer() cache.Store
	GetModuleInformer() cache.Store
	GetModuleConfigInformer() cache.Store
	GetInternalVMIInformer() cache.Store
}
