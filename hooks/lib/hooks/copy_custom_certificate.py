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
from lib.module import module
from lib.hooks.hook import Hook

class CopyCustomCertificatesHook(Hook):
    CUSTOM_CERTIFICATES_SNAPSHOT_NAME = "custom_certificates"
    def __init__(self,
                 module_name: str = None):
        super().__init__(module_name=module_name)
        self.queue = f"/modules/{self.module_name}/copy-custom-certificates"

    def generate_config(self) -> dict:
        return {
            "configVersion": "v1",
            "beforeHelm": 10,
            "kubernetes": [
                {
                    "name": self.CUSTOM_CERTIFICATES_SNAPSHOT_NAME,
                    "apiVersion": "v1",
                    "kind": "Secret",
                    "labelSelector": {
                        "matchExpressions": [
                            {
                                "key": "owner",
                                "operator": "NotIn",
                                "values": ["helm"]
                            }
                        ]
                    },
                    "namespace": {
                        "nameSelector": {
                            "matchNames": ["d8-system"]
                        }
                    },
                    "includeSnapshotsFrom": [self.CUSTOM_CERTIFICATES_SNAPSHOT_NAME],
                    "jqFilter": '{"name": .metadata.name, "data": .data}',
                    "queue": self.queue,
                    "keepFullObjectsInMemory": False
                },
            ]
        }

    def reconcile(self) -> Callable[[hook.Context], None]:
        def r(ctx: hook.Context) -> None:
            custom_certificates = {}
            for s in ctx.snapshots.get(self.CUSTOM_CERTIFICATES_SNAPSHOT_NAME, []):
                custom_certificates[s["filterResult"]["name"]] = s["filterResult"]["data"]
            if len(custom_certificates) == 0:
                return

            https_mode = module.get_https_mode(module_name=self.module_name,
                                            values=ctx.values)
            path = f"{self.module_name}.internal.customCertificateData"
            if https_mode != "CustomCertificate":
                self.delete_value(path, ctx.values)
                return

            raw_secret_name = module.get_values_first_defined(ctx.values,
                                                            f"{self.module_name}.https.customCertificate.secretName",
                                                            "global.modules.https.customCertificate.secretName")
            secret_name = str(raw_secret_name or "")
            secret_data = custom_certificates.get(secret_name)
            if secret_data is None:
                print(
                    f"Custom certificate secret name is configured, but secret d8-system/{secret_name} doesn't exist")
                return
            self.set_value(path, ctx.values, secret_data)
        return r