#!/usr/bin/env python3
#
# Copyright 2023 Flant JSC
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.


from lib.hooks.manage_tenant_secrets import ManageTenantSecretsHook
import common


def main():
    hook = ManageTenantSecretsHook(source_namespace=common.NAMESPACE,
                                   source_secret_name="virtualization-module-registry",
                                   module_name=common.MODULE_NAME,
                                   pod_labels_to_follow={
                                       "app": "containerized-data-importer",
                                       "app.kubernetes.io/managed-by": "cdi-controller-internal-virtualization"
                                   },
                                   destination_secret_labels={
                                       "heritage": "deckhouse",
                                       "kubevirt.deckhouse.io/cdi-registry-secret": "true",
                                       "deckhouse.io/registry-secret": "true"
                                   })
    hook.run()


if __name__ == "__main__":
    main()
