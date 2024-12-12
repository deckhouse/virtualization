#!/usr/bin/env python3
#
# Copyright 2024 Flant JSC
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


class ModuleConfigLabelHook(Hook):
    KIND="ModuleConfig"
    API_VERSION="deckhouse.io/v1alpha1"
    SNAPSHOT_NAME = "module-config-label"
    LABEL = "moduleconfig.deckhouse.io/name"

    def __init__(self, module_name: str):
        self.module_name = module_name

    def generate_config(self) -> dict:
        return {
            "configVersion": "v1",
            "kubernetes": [
                {
                    "name": self.SNAPSHOT_NAME,
                    "apiVersion": self.API_VERSION,
                    "kind": self.KIND,
                    "nameSelector": {
                        "matchNames": [self.module_name]
                    },
                    "includeSnapshotsFrom": [self.SNAPSHOT_NAME],
                    "jqFilter": '{"labels": .metadata.labels}',
                    "queue": f"/modules/{self.module_name}/{self.SNAPSHOT_NAME}",
                    "keepFullObjectsInMemory": False
                },
            ]
        }


    def reconcile(self) -> Callable[[hook.Context], None]:
        def r(ctx: hook.Context):
            labels = {}
            try:
                if (l := ctx.snapshots[self.SNAPSHOT_NAME][0]["filterResult"]["labels"]) and l is not None:
                    labels = l
            except (IndexError, KeyError):
                pass

            if labels.get(self.LABEL, "") == self.module_name:
                return

            ctx.kubernetes.merge_patch(
                kind=self.KIND,
                namespace="",
                name=self.module_name,
                patch={
                    "metadata": {
                        "labels": {
                            self.LABEL: self.module_name
                        }
                    }
                },
                apiVersion=self.API_VERSION
            )
        return r


if __name__ == "__main__":
    h = ModuleConfigLabelHook(common.MODULE_NAME)
    h.run()
