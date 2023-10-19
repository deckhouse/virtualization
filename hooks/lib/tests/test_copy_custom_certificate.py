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
from lib.hooks.copy_custom_certificate import CopyCustomCertificatesHook


MODULE_NAME = "test"
SECRET_NAME = "secretName"
SECRET_DATA = {
    "ca.crt": "CACRT",
    "tls.crt": "TLSCRT",
    "tls.key": "TLSKEY"
    }

hook = CopyCustomCertificatesHook(module_name=MODULE_NAME)

binding_context = [
    {
        "binding": "binding",
        "snapshots": {
            hook.CUSTOM_CERTIFICATES_SNAPSHOT_NAME: [
                {
                    "filterResult": {
                        "name": SECRET_NAME,
                        "data": SECRET_DATA
                    }
                },
                {
                    "filterResult": {
                        "name": "test",
                        "data": {}
                    }
                }
            ]
        }   
    }
]

values_add = {
    "global": {
        "modules": {
            "https": {
                "mode": "CustomCertificate",
                "customCertificate": {
                    "secretName": "test"
                }
            }
        }
    },
    MODULE_NAME: {
        "https": {
            "customCertificate": {
                "secretName": SECRET_NAME
            }
        },
        "internal": {}
    }
}

 
values_delete = {
    "global": {
        "modules": {
            "https": {
                "mode": "CertManager"
            }
        }
    },
    MODULE_NAME: {
        "internal": {
            "customCertificateData": SECRET_DATA
        }
    }
}


class TestCopyCustomCertificateAdd(testing.TestHook):
    def setUp(self):
        self.func            = hook.reconcile()
        self.bindind_context = binding_context
        self.values          = values_add
    def test_copy_custom_certificate_adding(self):
        self.hook_run()
        self.assertGreater(len(self.values[MODULE_NAME]["internal"].get("customCertificateData", {})), 0)
        self.assertEqual(self.values[MODULE_NAME]["internal"]["customCertificateData"], SECRET_DATA)

class TestCopyCustomCertificateDelete(testing.TestHook):
    def setUp(self):
        self.func            = hook.reconcile()
        self.bindind_context = binding_context
        self.values          = values_delete
    def test_copy_custom_certificate_deleting(self):
        self.hook_run()
        self.assertEqual(len(self.values[MODULE_NAME]["internal"].get("customCertificateData", {})), 0)


