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
from deckhouse.hook import Context
from lib.hooks.hook import Hook
from lib.password_generator import password_generator
import common
import lib.utils as utils

class Key:
    def __init__(self,
                 name: str,
                 value_path: str,
                 lenght: int = 64):
        self.name = name 
        self.value_path = value_path
        self.lenght = lenght
class GenerateSecretHook(Hook):
    SNAPSHOT_NAME   = "secrets"

    def __init__(self,
                 module_name: str = None):
        super().__init__(module_name=module_name)
        self.namespace = common.NAMESPACE
        self.secret_name = "dvcr-secrets"
        self.keys = (
            Key(name="passwordRW", 
                value_path=f"{self.module_name}.internal.dvcr.passwordRW", 
                lenght=32),
            Key(name="salt",
                value_path=f"{self.module_name}.internal.dvcr.salt")
        )     

    def generate_config(self) -> dict:
        return {
            "configVersion": "v1",
            "beforeHelm": 5,
            "kubernetes": [
                {
                    "name": self.SNAPSHOT_NAME,
                    "apiVersion": "v1",
                    "kind": "Secret",
                    "nameSelector": {
                        "matchNames": [self.secret_name]
                    },
                    "namespace": {
                        "nameSelector": {
                            "matchNames": [self.namespace]
                        }
                    },
                    "includeSnapshotsFrom": [self.SNAPSHOT_NAME],
                    "jqFilter": '{"data": .data}',
                    "queue": f"/modules/{self.module_name}/generate_secrets",
                    "keepFullObjectsInMemory": False
                },
            ]
        }
    
    def reconcile(self) -> Callable[[Context], None]:
        def r(ctx: Context):
            for key in self.keys:
                try:
                    key_from_secret = ctx.snapshots[self.SNAPSHOT_NAME][0]["filterResult"]["data"][key.name]
                    self.set_value(key.value_path, ctx.values, key_from_secret)
                except (IndexError, KeyError):
                    print(f"Generate new key {key.name} for secret {self.secret_name}.")
                    genkey = utils.base64_encode_from_str(password_generator.alpha_num_symbols(key.lenght))
                    self.set_value(key.value_path, ctx.values, genkey)
        return r

if __name__ == "__main__":
    hook = GenerateSecretHook()
    hook.run()