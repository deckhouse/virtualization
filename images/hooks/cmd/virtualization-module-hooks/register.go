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

package main

import (
	_ "github.com/deckhouse/virtualization/hooks/pkg/hooks/ca-discovery"
	_ "github.com/deckhouse/virtualization/hooks/pkg/hooks/copy-custom-certificate"
	_ "github.com/deckhouse/virtualization/hooks/pkg/hooks/discover-kube-apiserver-feature-gates"
	_ "github.com/deckhouse/virtualization/hooks/pkg/hooks/discovery-clusterip-service-for-dvcr"
	_ "github.com/deckhouse/virtualization/hooks/pkg/hooks/discovery-workload-nodes"
	_ "github.com/deckhouse/virtualization/hooks/pkg/hooks/dvcr-garbage-collection"
	_ "github.com/deckhouse/virtualization/hooks/pkg/hooks/generate-secret-for-dvcr"
	_ "github.com/deckhouse/virtualization/hooks/pkg/hooks/install-vmclass-generic"
	_ "github.com/deckhouse/virtualization/hooks/pkg/hooks/migrate-virthandler-kvm-node-labels"
	_ "github.com/deckhouse/virtualization/hooks/pkg/hooks/migrate-vm-network-interface-ids"
	_ "github.com/deckhouse/virtualization/hooks/pkg/hooks/migration-config"
	_ "github.com/deckhouse/virtualization/hooks/pkg/hooks/tls-certificates-api"
	_ "github.com/deckhouse/virtualization/hooks/pkg/hooks/tls-certificates-api-proxy"
	_ "github.com/deckhouse/virtualization/hooks/pkg/hooks/tls-certificates-controller"
	_ "github.com/deckhouse/virtualization/hooks/pkg/hooks/tls-certificates-dvcr"
	_ "github.com/deckhouse/virtualization/hooks/pkg/hooks/validate-module-config"
)
