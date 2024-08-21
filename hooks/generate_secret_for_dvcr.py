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
from deckhouse.hook import Context
from lib.hooks.hook import Hook
from lib.password_generator import password_generator, htpasswd as hd
import common
import lib.utils as utils


class KeyHtpasswd:
    def __init__(self,
                 name: str,
                 username: str,
                 value_path: str):
        self.name = name
        self.username = username
        self.value_path = value_path


class Key:
    def __init__(self,
                 name: str,
                 value_path: str,
                 length: int = 64,
                 htpasswd: KeyHtpasswd = None):
        self.name = name
        self.value_path = value_path
        self.length = length
        self.htpasswd = htpasswd


class GenerateSecretHook(Hook):
    SNAPSHOT_NAME = "secrets"

    def __init__(self,
                 *keys: Key,
                 secret_name: str,
                 namespace: str,
                 module_name: str,
                 ):
        self.module_name = module_name
        self.keys = keys
        self.secret_name = secret_name
        self.namespace = namespace
        self.queue = f"/modules/{self.module_name}/generate_secrets"

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
                    "queue": self.queue,
                    "keepFullObjectsInMemory": False
                },
            ]
        }

    def reconcile(self) -> Callable[[Context], None]:
        def r(ctx: Context):
            for key in self.keys:
                try:
                    key_base64 = ctx.snapshots[self.SNAPSHOT_NAME][0]["filterResult"]["data"][key.name]
                    self.set_value(key.value_path, ctx.values, key_base64)
                except (IndexError, KeyError):
                    print(
                        f"Generate new key {key.name} for secret={self.secret_name}.")
                    genkey = utils.base64_encode_from_str(
                        password_generator.alpha_num(key.length))
                    self.set_value(key.value_path, ctx.values, genkey)
            for key in self.keys:
                if key.htpasswd is None:
                    continue
                password = utils.base64_decode(
                    self.get_value(key.value_path, ctx.values))
                htpasswd = hd.Htpasswd(
                    username=key.htpasswd.username, password=password)
                regenerate = False
                try:
                    htpasswd_base64 = ctx.snapshots[self.SNAPSHOT_NAME][0]["filterResult"]["data"][key.htpasswd.name]
                    if htpasswd.validate(utils.base64_decode(htpasswd_base64)):
                        self.set_value(key.htpasswd.value_path,
                                       ctx.values, htpasswd_base64)
                        continue
                    regenerate = True
                except (IndexError, KeyError):
                    regenerate = True
                if regenerate:
                    print(
                        f"Generate new htpasswd for key={key.name} and secret={self.secret_name}.")
                    encoded_htpasswd = utils.base64_encode_from_str(
                        htpasswd.generate())
                    self.set_value(key.htpasswd.value_path,
                                   ctx.values, encoded_htpasswd)
        return r


if __name__ == "__main__":
    hook = GenerateSecretHook(
        Key(name="passwordRW",
            value_path=f"{common.MODULE_NAME}.internal.dvcr.passwordRW",
            length=32,
            htpasswd=KeyHtpasswd(
                name="htpasswd",
                username="admin",
                value_path=f"{common.MODULE_NAME}.internal.dvcr.htpasswd",
            )
            ),
        Key(name="salt",
            value_path=f"{common.MODULE_NAME}.internal.dvcr.salt"),
        secret_name="dvcr-secrets",
        namespace=common.NAMESPACE,
        module_name=common.MODULE_NAME
    )
    hook.run()
