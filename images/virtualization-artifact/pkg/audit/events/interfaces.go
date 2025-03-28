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
)

// import kubecache "k8s.io/client-go/tools/cache"

//go:generate moq -rm -out mock.go . TTLCache Indexer InformerList

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
	GetClient() *kubernetes.Clientset
}

type Indexer interface {
	GetByKey(string) (any, bool, error)
}

type InformerList interface {
	GetVMInformer() Indexer
	GetVDInformer() Indexer
	GetVMOPInformer() Indexer
	GetPodInformer() Indexer
	GetNodeInformer() Indexer
	GetModuleInformer() Indexer
	GetModuleConfigInformer() Indexer
	GetInternalVMIInformer() Indexer
}
