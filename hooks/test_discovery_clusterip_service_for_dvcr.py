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

from discovery_clusterip_service_for_dvcr import DiscoveryClusterIPServiceHook
from lib.tests import testing

CLUSTER_IP_ADD = "10.10.10.10"
CLUSTER_IP_CHANGE = "11.11.11.11"

hook = DiscoveryClusterIPServiceHook(module_name="test")

binding_context_add = [
    {
        "binding": hook.SNAPSHOT_NAME,
        "snapshots": {
            hook.SNAPSHOT_NAME: [
                {
                    "filterResult": {
                        "clusterIP": CLUSTER_IP_ADD
                    }
                }
            ]
        }
    }
]

initial_values_add = {
    "test": {
        "internal": {}
    }
}

binding_context_delete = [
    {
        "binding": hook.SNAPSHOT_NAME,
        "snapshots": {
            hook.SNAPSHOT_NAME: []
        }
    }
]

binding_context_change = [
    {
        "binding": hook.SNAPSHOT_NAME,
        "snapshots": {
            hook.SNAPSHOT_NAME: [
                {
                    "filterResult": {
                        "clusterIP": CLUSTER_IP_CHANGE
                    }
                }
            ]
        }
    }
]

initial_values_delete_or_change = {
    "test": {
        "internal": {
            "dvcr": {
                "serviceIP": CLUSTER_IP_ADD
            }
        }
    }
}


class TestClusterIPAdd(testing.TestHook):
    def setUp(self):
        self.func = hook.reconcile()
        self.bindind_context = binding_context_add
        self.values = initial_values_add

    def test_adding(self):
        self.hook_run()
        self.assertEqual(self.values["test"]["internal"]["dvcr"]["serviceIP"],
                         CLUSTER_IP_ADD)


class TestClusterIPChange(testing.TestHook):
    def setUp(self):
        self.func = hook.reconcile()
        self.bindind_context = binding_context_change
        self.values = initial_values_delete_or_change

    def test_changing(self):
        self.hook_run()
        self.assertEqual(self.values["test"]["internal"]["dvcr"]["serviceIP"],
                         CLUSTER_IP_CHANGE)


class TestClusterIPDelete(testing.TestHook):
    def setUp(self):
        self.func = hook.reconcile()
        self.bindind_context = binding_context_delete
        self.values = initial_values_delete_or_change

    def test_deleting(self):
        self.hook_run()
        self.assertEqual(self.values["test"]["internal"]["dvcr"].get("serviceIP"),
                         None)
