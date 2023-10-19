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
from lib.hooks.manage_tenant_secrets import ManageTenantSecretsHook

hook = ManageTenantSecretsHook(source_namespace="source_namespace",
                               source_secret_name="secret_name",
                               pod_labels_to_follow={"app": "test"},
                               destination_secret_labels={"test":"test"},
                               module_name="test")

binding_context = [
    {
        "binding": "binding",
        "snapshots": {
            hook.POD_SNAPSHOT_NAME: [
                {
                    "filterResult": {
                        "namespace": "pod-namespace1"  ## Create secret  
                    }
                },
                {
                    "filterResult": {
                        "namespace": "pod-namespace2" ## Don't create secret, because ns has deletionTimestamp
                    }
                }
            ],
            hook.SECRETS_SNAPSHOT_NAME: [
                {
                    "filterResult": {
                        "data": {"test": "test"},
                        "namespace": "source_namespace", 
                        "type": "Opaque"
                    }
                },
                {
                    "filterResult": {
                        "data": {"test": "test"},
                        "namespace": "pod-namespace3", ## Delete secret, because namespace pod-namespace3 hasn't pods
                        "type": "Opaque"
                    }
                },
            ],
            hook.NAMESPACE_SNAPSHOT_NAME: [
                {
                    "filterResult": {
                        "name": "source_namespace",
                        "isTerminating": False
                    }
                }, 
                {
                    "filterResult": {
                        "name": "pod-namespace1",
                        "isTerminating": False
                    }
                },  
                {
                    "filterResult": {
                        "name": "pod-namespace2",
                        "isTerminating": True
                    }
                },
                {
                    "filterResult": {
                        "name": "pod-namespace3",
                        "isTerminating": False
                    }
                }, 
            ]
        }   
    }
]
 
class TestManageSecrets(testing.TestHook):
    def setUp(self):
        self.func            = hook.reconcile()
        self.bindind_context = binding_context
        self.values          = {}
    def test_manage_secrets(self):
        self.hook_run()
        self.assertEqual(len(self.kube_resources), 1)
        self.assertEqual(self.kube_resources[0]["kind"], "Secret")
        self.assertEqual(self.kube_resources[0]["metadata"]["name"], "secret_name")
        self.assertEqual(self.kube_resources[0]["metadata"]["namespace"], "pod-namespace1")
        self.assertEqual(self.kube_resources[0]["type"], "Opaque")
        self.assertEqual(self.kube_resources[0]["data"], {'test': 'test'})
        self.assertEqual(self.kube_resources[0]["metadata"]["labels"], {'test': 'test'})

