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
import lib.utils as utils
from generate_secret_for_dvcr import GenerateSecretHook

MODULE_NAME = "test"
hook = GenerateSecretHook(module_name=MODULE_NAME)

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

binding_context_regenerate_key2= [
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
            "dvcr": {
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
        return self.values[MODULE_NAME]["internal"]["dvcr"].get(key, "")
    
class TestGenerateSecretALL(TestGenerateKeys):
    def setUp(self):
        self.func            = hook.reconcile()
        self.bindind_context = binding_context_generate_all
        self.values          = initial_values_generate_all
    def test_generate_all(self):
        self.hook_run()
        self.key_test(self.get_key(hook.keys[0].name), hook.keys[0].lenght)
        self.key_test(self.get_key(hook.keys[1].name), hook.keys[1].lenght)

class TestGenerateSecretKey1(TestGenerateKeys):
    def setUp(self):
        self.func            = hook.reconcile()
        self.bindind_context = binding_context_regenerate_key1
        self.values          = initial_values_regenerate_something
    def test_generate_passwordRW(self):
        self.hook_run()
        self.key_test(self.get_key(hook.keys[0].name), hook.keys[0].lenght)

class TestGenerateSecretKey2(TestGenerateKeys):
    def setUp(self):
        self.func            = hook.reconcile()
        self.bindind_context = binding_context_regenerate_key2
        self.values          = initial_values_regenerate_something
    def test_generate_salt(self):
        self.hook_run()
        self.key_test(self.get_key(hook.keys[1].name), hook.keys[1].lenght)
