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


class DiscoveryClusterIPServiceHook(Hook):
    SNAPSHOT_NAME = "discovery-service"

    def __init__(self, module_name: str = None):
        super().__init__(module_name=module_name)
        self.namespace = common.NAMESPACE
        self.service_name = "dvcr"
        self.value_path = f"{self.module_name}.internal.dvcr.serviceIP"

    def generate_config(self) -> dict:
        return {
            "configVersion": "v1",
            "beforeHelm": 5,
            "kubernetes": [
                {
                    "name": self.SNAPSHOT_NAME,
                    "apiVersion": "v1",
                    "kind": "Service",
                    "nameSelector": {
                        "matchNames": [self.service_name]
                    },
                    "namespace": {
                        "nameSelector": {
                            "matchNames": [self.namespace]
                        }
                    },
                    "includeSnapshotsFrom": [self.SNAPSHOT_NAME],
                    "jqFilter": '{"clusterIP": .spec.clusterIP}',
                    "queue": f"/modules/{self.module_name}/discovery-service",
                    "keepFullObjectsInMemory": False
                },
            ]
        }

    def reconcile(self) -> Callable[[hook.Context], None]:
        def r(ctx: hook.Context):
            services = [s["filterResult"]["clusterIP"]
                        for s in ctx.snapshots.get(self.SNAPSHOT_NAME, [])]
            if len(services) == 0:
                print(
                    f"Service dvcr not found. Delete value from {self.value_path}.")
                self.delete_value(self.value_path, ctx.values)
            else:
                print(f"Set {services[0]} to {self.value_path}.")
                self.set_value(self.value_path, ctx.values, services[0])
        return r


if __name__ == "__main__":
    h = DiscoveryClusterIPServiceHook()
    h.run()
