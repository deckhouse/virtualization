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

package coordination

// GroupName is the group name for resources defined in this package.
//
// It is a subgroup of internal.virtualization.deckhouse.io rather than that group itself: kube-api-rewriter
// renames kubevirt.io to internal.virtualization.deckhouse.io as a whole group, so a resource placed there
// would be presented to virtualization-controller under kubevirt.io in discovery.
const GroupName = "coordination.internal.virtualization.deckhouse.io"
