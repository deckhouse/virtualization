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
import unittest
import jsonpatch
import kubernetes_validate
import jsonschema

class TestHook(unittest.TestCase):
    kube_resources = []
    kube_version = "1.28"
    def setUp(self):
        self.bindind_context = []
        self.values = {}
        self.func = None 
    
    def tearDown(self):
        pass

    def hook_run(self, validate_kube_resources: bool = True) -> None:
        out = hook.testrun(func=self.func,
                           binding_context=self.bindind_context,
                           initial_values=self.values)
        for patch in out.values_patches.data:
            self.values = jsonpatch.apply_patch(self.values, [patch])

        deletes = ("Delete", "DeleteInBackground", "DeleteNonCascading")
        for kube_operation in out.kube_operations.data:
            if kube_operation["operation"] in deletes:
                continue
            obj = kube_operation["object"]
            if validate_kube_resources:
                try:
                    ## TODO Validate CRD
                    kubernetes_validate.validate(obj, self.kube_version, strict=True)
                    self.kube_resources.append(obj)
                except (kubernetes_validate.SchemaNotFoundError,
                        kubernetes_validate.InvalidSchemaError,
                        kubernetes_validate.ValidationError,
                        jsonschema.RefResolutionError) as e:
                    self.fail(f"Object is not valid. Raised an exception: {e} ")
            else:
                self.kube_resources.append(obj)

