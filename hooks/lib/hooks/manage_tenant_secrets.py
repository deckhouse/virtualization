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

from deckhouse import hook
from typing import Callable
from lib.hooks.hook import Hook

class ManageTenantSecretsHook(Hook):
    POD_SNAPSHOT_NAME = "pods"
    SECRETS_SNAPSHOT_NAME = "secrets"
    NAMESPACE_SNAPSHOT_NAME = "namespaces"

    def __init__(self, 
                 source_namespace: str,
                 source_secret_name: str,
                 pod_labels_to_follow: dict,
                 destination_secret_labels: dict = {},
                 module_name: str = None):
        super().__init__(module_name=module_name)
        self.source_namespace = source_namespace
        self.source_secret_name = source_secret_name
        self.pod_labels_to_follow = pod_labels_to_follow
        self.destination_secret_labels = destination_secret_labels
        self.module_name = module_name
        self.queue = f"/modules/{module_name}/manage-tenant-secrets"

    def generate_config(self) -> dict:
        return {
            "configVersion": "v1",
            "kubernetes": [
                {
                    "name": self.POD_SNAPSHOT_NAME,
                    "apiVersion": "v1",
                    "kind": "Pod",
                    "includeSnapshotsFrom": [
                        self.POD_SNAPSHOT_NAME,
                        self.SECRETS_SNAPSHOT_NAME,
                        self.NAMESPACE_SNAPSHOT_NAME
                    ],
                    "labelSelector": {
                        "matchLabels": self.pod_labels_to_follow
                    },
                    "jqFilter": '{"namespace": .metadata.namespace}',
                    "queue": self.queue,
                    "keepFullObjectsInMemory": False
                },
                {
                    "name": self.SECRETS_SNAPSHOT_NAME,
                    "apiVersion": "v1",
                    "kind": "Secret",
                    "includeSnapshotsFrom": [
                        self.POD_SNAPSHOT_NAME,
                        self.SECRETS_SNAPSHOT_NAME,
                        self.NAMESPACE_SNAPSHOT_NAME
                    ],
                    "nameSelector": {
                        "matchNames": [self.source_secret_name]
                    },
                    "jqFilter": '{"data": .data, "namespace": .metadata.namespace, "type": .type}', 
                    "queue": self.queue,
                    "keepFullObjectsInMemory": False
                },
                {
                    "name": self.NAMESPACE_SNAPSHOT_NAME,
                    "apiVersion": "v1",
                    "kind": "Secret",
                    "includeSnapshotsFrom": [
                        self.POD_SNAPSHOT_NAME,
                        self.SECRETS_SNAPSHOT_NAME,
                        self.NAMESPACE_SNAPSHOT_NAME
                    ],
                    "jqFilter": '{"name": .metadata.name, "isTerminating": any(.metadata; .deletionTimestamp != null)}', 
                    "queue": self.queue,
                    "keepFullObjectsInMemory": False
                }
            ]
        }

    def generate_secret(self, namespace: str, data: dict, secret_type: str) -> dict:
        return {
            "apiVersion": "v1",
            "kind": "Secret",
            "metadata": {
                "name": self.source_secret_name,
                "namespace": namespace,
                "labels": self.destination_secret_labels
            },
            "data": data,
            "type": secret_type
        }
    
    def reconcile(self) -> Callable[[hook.Context], None]:
        def r(ctx: hook.Context) -> None:
            pod_namespaces = set([p["filterResult"]["namespace"] for p in ctx.snapshots.get(self.POD_SNAPSHOT_NAME, [])])
            secrets = ctx.snapshots.get(self.SECRETS_SNAPSHOT_NAME, [])
            for ns in ctx.snapshots.get(self.NAMESPACE_SNAPSHOT_NAME, []):
                if  ns["filterResult"]["isTerminating"]:
                    pod_namespaces.discard(ns["filterResult"]["name"])
            data, secret_type, secrets_by_ns = "", "", {}
            for s in secrets:
                if s["filterResult"]["namespace"] == self.source_namespace:
                    data = s["filterResult"]["data"]
                    secret_type = s["filterResult"]["type"]
                    continue
                secrets_by_ns[s["filterResult"]["namespace"]] = s["filterResult"]["data"]

            if len(data) == 0 or len(secret_type) == 0:
                print(f"Registry secret {self.source_namespace}/{self.source_secret_name} not found. Skip")
                return

            for ns in pod_namespaces:
                secret_data = secrets_by_ns.get(ns, "")
                if (secret_data != data) and (ns != self.source_namespace):
                    secret = self.generate_secret(namespace=ns,
                                                  data=data,
                                                  secret_type=secret_type)
                    print(f"Create secret {ns}/{self.source_secret_name}.")
                    ctx.kubernetes.create_or_update(secret)
            for ns in secrets_by_ns:
                if (ns in pod_namespaces) or (ns == self.source_namespace):
                    continue
                print(f"Delete secret {ns}/{self.source_secret_name}.")
                ctx.kubernetes.delete(kind="Secret",
                                      namespace=ns,
                                      name=self.source_secret_name)
        return r

