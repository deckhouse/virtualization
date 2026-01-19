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
	_ "hooks/pkg/hooks/ca-discovery"
	_ "hooks/pkg/hooks/copy-custom-certificate"
	_ "hooks/pkg/hooks/discovery-clusterip-service-for-dvcr"
	_ "hooks/pkg/hooks/discovery-workload-nodes"
	_ "hooks/pkg/hooks/drop-openshift-labels"
	_ "hooks/pkg/hooks/dvcr-garbage-collection"
	_ "hooks/pkg/hooks/generate-secret-for-dvcr"
	_ "hooks/pkg/hooks/install-vmclass-generic"
	_ "hooks/pkg/hooks/migrate-virthandler-kvm-node-labels"
	_ "hooks/pkg/hooks/parallel-outbound-migrations-per-node"
	_ "hooks/pkg/hooks/tls-certificates-api"
	_ "hooks/pkg/hooks/tls-certificates-api-proxy"
	_ "hooks/pkg/hooks/tls-certificates-controller"
	_ "hooks/pkg/hooks/tls-certificates-dvcr"
	_ "hooks/pkg/hooks/validate-module-config"
)
