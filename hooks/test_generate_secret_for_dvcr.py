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

from lib.tests import testing
from lib.password_generator import htpasswd as hd
import lib.utils as utils
from generate_secret_for_dvcr import GenerateSecretHook, Key, KeyHtpasswd

MODULE_NAME = "test"

hook = GenerateSecretHook(
    Key(name="key1",
        value_path=f"{MODULE_NAME}.internal.test.key1",
        length=32,
        htpasswd=KeyHtpasswd(
            name="htpasswd",
            username="admin",
            value_path=f"{MODULE_NAME}.internal.test.htpasswd",
        )
        ),
    Key(name="key2",
        value_path=f"{MODULE_NAME}.internal.test.key2"),
    secret_name=MODULE_NAME,
    namespace=MODULE_NAME,
    module_name=MODULE_NAME
)

binding_context_generate_all = [
    {
        "binding": hook.SNAPSHOT_NAME,
        "snapshots": {
            hook.SNAPSHOT_NAME: []
        }
    }
]


initial_values_generate_all = {
    MODULE_NAME: {
        "internal": {}
    }
}

binding_context_regenerate_key1 = [
    {
        "binding": hook.SNAPSHOT_NAME,
        "snapshots": {
            hook.SNAPSHOT_NAME: [
                {
                    "filterResult": {
                        "data": {
                            hook.keys[1].name: ""
                        }
                    }
                }
            ]
        }
    }
]

binding_context_regenerate_key2 = [
    {
        "binding": hook.SNAPSHOT_NAME,
        "snapshots": {
            hook.SNAPSHOT_NAME: [
                {
                    "filterResult": {
                        "data": {
                            hook.keys[0].name: ""
                        }
                    }
                }
            ]
        }
    }
]

initial_values_regenerate_something = {
    MODULE_NAME: {
        "internal":  {
            "test": {
                hook.keys[0].name: "",
                hook.keys[1].name: ""
            }
        }
    }
}


class TestGenerateKeys(testing.TestHook):
    def key_test(self, key: str, lenght: int):
        self.assertGreater(len(key), 0)
        self.assertTrue(utils.is_base64(key))
        self.assertEqual(len(utils.base64_decode(key)), lenght)

    def get_key(self, key: str) -> str:
        return self.values[MODULE_NAME]["internal"]["test"].get(key, "")


class TestGenerateSecretALL(TestGenerateKeys):
    def setUp(self):
        self.func = hook.reconcile()
        self.bindind_context = binding_context_generate_all
        self.values = initial_values_generate_all

    def test_generate_all(self):
        self.hook_run()
        self.key_test(self.get_key(hook.keys[0].name), hook.keys[0].length)
        self.key_test(self.get_key(hook.keys[1].name), hook.keys[1].length)


class TestGenerateSecretKey1(TestGenerateKeys):
    def setUp(self):
        self.func = hook.reconcile()
        self.bindind_context = binding_context_regenerate_key1
        self.values = initial_values_regenerate_something

    def test_generate_key1(self):
        self.hook_run()
        self.key_test(self.get_key(hook.keys[0].name), hook.keys[0].length)

    def test_generate_htpasswd(self):
        self.hook_run()
        htpasswd_base64 = self.get_key(hook.keys[0].htpasswd.name)
        self.assertGreater(len(htpasswd_base64), 0)
        self.assertTrue(utils.is_base64(htpasswd_base64))
        password_base64 = self.get_key(hook.keys[0].name)
        self.key_test(password_base64, hook.keys[0].length)
        htpasswd = hd.Htpasswd(
            hook.keys[0].htpasswd.username, utils.base64_decode(password_base64))
        self.assertTrue(htpasswd.validate(
            utils.base64_decode(htpasswd_base64)))


class TestGenerateSecretKey2(TestGenerateKeys):
    def setUp(self):
        self.func = hook.reconcile()
        self.bindind_context = binding_context_regenerate_key2
        self.values = initial_values_regenerate_something

    def test_generate_key2(self):
        self.hook_run()
        self.key_test(self.get_key(hook.keys[1].name), hook.keys[1].length)
