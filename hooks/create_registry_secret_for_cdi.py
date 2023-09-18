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

#!/usr/bin/env python3
from deckhouse import hook

CDI_POD_SNAPSHOT          = "cdi_pods"
SECRET_SNAPSHOT           = "virt_registry_secrets_namespaces"
VIRT_REGISTRY_SNAPSHOT    = "virt_registry_secret"
VIRT_REGISTRY_SECRET_NAME = "virtualization-module-registry"
NAMESPACE                 = "d8-virtualization"
QUEUE                     = "/modules/virtualization/containerized-data-importer/registry-secrets"

config = f"""
configVersion: v1
kubernetes:
- name: {CDI_POD_SNAPSHOT}
  apiVersion: v1
  kind: Pod
  includeSnapshotsFrom: 
  - "{CDI_POD_SNAPSHOT}" 
  - "{SECRET_SNAPSHOT}"
  - "{VIRT_REGISTRY_SNAPSHOT}"
  labelSelector:
    matchLabels:
      app: containerized-data-importer
      app.kubernetes.io/managed-by: cdi-controller
  queue: "{QUEUE}"
- name: {SECRET_SNAPSHOT}
  apiVersion: v1
  kind: Secret
  includeSnapshotsFrom: 
  - "{CDI_POD_SNAPSHOT}" 
  - "{SECRET_SNAPSHOT}"
  - "{VIRT_REGISTRY_SNAPSHOT}"
  nameSelector:
    matchNames:
    - "{VIRT_REGISTRY_SECRET_NAME}"
  queue: "{QUEUE}"
- name: {VIRT_REGISTRY_SNAPSHOT}
  apiVersion: v1
  kind: Secret
  includeSnapshotsFrom: 
  - "{CDI_POD_SNAPSHOT}" 
  - "{SECRET_SNAPSHOT}"
  - "{VIRT_REGISTRY_SNAPSHOT}"
  nameSelector:
    matchNames:
    - "{VIRT_REGISTRY_SECRET_NAME}"
  namespace:
    nameSelector:
      matchNames: ["{NAMESPACE}"]
  queue: "{QUEUE}"
"""

class RegistrySecret:
    def __init__(self, ns: str, conf: str):
        self.namespace = ns
        self.config = conf

def apply_registry_secret_filter(secrets: list) -> list:
    result = []
    for secret in secrets:
        ns = secret["object"]["metadata"]["namespace"]
        data = secret["object"]["data"].get(".dockerconfigjson")
        if data is None:
            print(f"registry auth conf is not in registry secret {secret['metadata']['name']}")
            continue
        result.append(RegistrySecret(ns=ns, conf=str(data)))
    return result

def apply_cdi_pod_filter(pods: list) -> set:
    return set([p["object"]["metadata"]["namespace"] for p in pods])

def prepare_virt_registry_secret(ns: str, cfg: str) -> dict:
    return {
        "apiVersion": "v1",
        "kind": "Secret",
        "metadata": {
            "labels": {
                "heritage": "deckhouse",
                "kubevirt.deckhouse.io/cdi-registry-secret": "true",
                "deckhouse.io/registry-secret": "true"
            },
            "name": VIRT_REGISTRY_SECRET_NAME,
            "namespace": ns
        },
        "data": {
            ".dockerconfigjson": cfg
        },
        "type": "kubernetes.io/dockerconfigjson"
    }

def main(ctx: hook.Context):
    secrets = secrets = apply_registry_secret_filter(ctx.snapshots.get(SECRET_SNAPSHOT, []))
    registry_secrets = apply_registry_secret_filter(ctx.snapshots.get(VIRT_REGISTRY_SNAPSHOT, []))
    namespaces = apply_cdi_pod_filter(ctx.snapshots.get(CDI_POD_SNAPSHOT, set()))
    
    if len(registry_secrets) == 0:
        print("Registry secret not found. Skip")
        return
    
    registry_cfg = registry_secrets[0].config

    secrets_by_ns = {s.namespace:s.config for s in secrets}

    for ns in namespaces:
        secret_content = secrets_by_ns.get(ns, "")
        if (secret_content != registry_cfg) and (ns != NAMESPACE):
            secret = prepare_virt_registry_secret(ns=ns, cfg=registry_cfg)
            ctx.kubernetes.create_or_update(secret)

    for ns in secrets_by_ns:
        if (ns in namespaces) or (ns == NAMESPACE):
            continue
        ctx.kubernetes.delete(kind="Secret", 
                              namespace=ns, 
                              name=VIRT_REGISTRY_SECRET_NAME)

if __name__ == "__main__":
    hook.run(main, config=config)

