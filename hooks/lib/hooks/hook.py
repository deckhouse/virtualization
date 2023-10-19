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
from lib.module import values as module_values
import yaml

class Hook:
    def __init__(self, module_name: str = None) -> None:
        self.module_name = self.get_module_name(module_name)

    def generate_config(self):
        pass

    @staticmethod
    def get_value(path: str, values: dict, default=None):
        return module_values.get_value(path, values, default)
    
    @staticmethod
    def set_value(path: str, values: dict, value: str) -> None:
        return module_values.set_value(path, values, value)
    
    @staticmethod
    def delete_value(path: str, values: dict) -> None:
        return module_values.delete_value(path, values)
    
    @staticmethod
    def get_module_name(module_name: str) -> str:
        if module_name is not None:
            return module_name
        return module.get_module_name()

    def reconcile(self) -> Callable[[hook.Context], None]:
        def r(ctx: hook.Context) -> None:
            pass
        return r
    
    def run(self) -> None:
        conf = self.generate_config()
        if isinstance(conf, dict):
            conf = yaml.dump(conf)
        hook.run(func=self.reconcile(), config=conf)
