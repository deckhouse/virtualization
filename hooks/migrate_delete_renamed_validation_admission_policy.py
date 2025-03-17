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

from typing import Callable
from deckhouse import hook
from lib.hooks.hook import Hook
import common


# MigrateDeleteRenamedVAPResources deletes orphan resources after renaming them in Kubevirt.
# Deletion is required to not conflict with the original Kubevirt installation.
# AfterHelm binding is used to minimize risk of resources recreating by the old version of the virt-operator.
class MigrateDeleteRenamedVAPResources(Hook):
    BINDING_SNAPSHOT_NAME = "validating_admission_policy"
    POLICY_SNAPSHOT_NAME = "validating_admission_policy_binding"

    def __init__(self, module_name: str):
        self.module_name = module_name
        self.vapolicy_name = "kubevirt-node-restriction-policy"
        self.vapolicy_binding_name = "kubevirt-node-restriction-binding"
        self.managed_by_label = "app.kubernetes.io/managed-by"
        self.managed_by_label_value = "virt-operator-internal-virtualization"

    def generate_config(self) -> dict:
        return {
            "configVersion": "v1",
            "afterHelm": 5,
            "kubernetes": [
                {
                    "name": self.POLICY_SNAPSHOT_NAME,
                    "apiVersion": "admissionregistration.k8s.io/v1beta1",
                    "kind": "ValidatingAdmissionPolicy",
                    "nameSelector": {
                        "matchNames": [self.vapolicy_name]
                    },
                    "group": "all",
                    "jqFilter": '{"name": .metadata.name, "kind": .kind, "labels": .metadata.labels}',
                    "queue": f"/modules/{self.module_name}/vap-resources",
                    "keepFullObjectsInMemory": False,
                    "executeHookOnSynchronization": False,
                    "executeHookOnEvent": []
                },
                {
                    "name": self.BINDING_SNAPSHOT_NAME,
                    "apiVersion": "admissionregistration.k8s.io/v1beta1",
                    "kind": "ValidatingAdmissionPolicyBinding",
                    "nameSelector": {
                        "matchNames": [self.vapolicy_binding_name]
                    },
                    "group": "all",
                    "jqFilter": '{"name": .metadata.name, "kind": .kind, "labels": .metadata.labels}',
                    "queue": f"/modules/{self.module_name}/vap-resources",
                    "keepFullObjectsInMemory": False,
                    "executeHookOnSynchronization": False,
                    "executeHookOnEvent": []
                },
            ]
        }

    def reconcile(self) -> Callable[[hook.Context], None]:
        def r(ctx: hook.Context):
            found_deprecated = 0
            for snapshot_name in [self.POLICY_SNAPSHOT_NAME, self.BINDING_SNAPSHOT_NAME]:
                for s in ctx.snapshots.get(snapshot_name, []):
                    labels = s["filterResult"]["labels"]
                    if not self.managed_by_label in labels:
                        continue
                    if labels[self.managed_by_label] == self.managed_by_label_value:
                        ++found_deprecated
                        # Delete
                        name = s["filterResult"]["name"]
                        kind = s["filterResult"]["kind"]
                        print(f"Delete deprecated {kind} {name}.")
                        ctx.kubernetes.delete(kind=kind,
                                          namespace='',
                                          name=name)
            if found_deprecated == 0:
                print("No deprecated resources found, migration not required.")
        return r


if __name__ == "__main__":
    h = MigrateDeleteRenamedVAPResources(common.MODULE_NAME)
    h.run()
