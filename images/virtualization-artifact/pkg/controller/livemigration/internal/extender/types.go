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

package extender

import (
	corev1 "k8s.io/api/core/v1"
)

// Wire types of the kube-scheduler extender protocol (k8s.io/kube-scheduler/extender/v1). They
// are copied rather than imported to keep a new staging module out of the workspace; the protocol
// is versioned and stable. The upstream types carry no JSON tags, so neither do these.

// Args is the request of the filter and prioritize verbs. The scheduler sends NodeNames only,
// because Deckhouse registers every extender with nodeCacheCapable: true.
type Args struct {
	Pod       *corev1.Pod
	NodeNames *[]string
}

// FilterResult is the response of the filter verb.
type FilterResult struct {
	NodeNames *[]string
	Error     string
}

// HostPriority is a node score in the response of the prioritize verb, from 0 to 10.
type HostPriority struct {
	Host  string
	Score int64
}
