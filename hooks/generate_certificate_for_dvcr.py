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


from lib.hooks.internal_tls import GenerateCertificateHook
from lib.module import values as module_values
from deckhouse import hook
from typing import Callable
import common

def main():
    hook = GenerateCertificateHook(
        cn="dvcr",
        sansGenerator=sans_generator([
            "dvcr",
            f"dvcr.{common.NAMESPACE}",
            f"dvcr.{common.NAMESPACE}.svc"]),
        namespace=common.NAMESPACE,
        tls_secret_name="dvcr-tls",
        values_path_prefix=f"{common.MODULE_NAME}.internal.dvcr.cert",
        before_hook_check=hook_check)

    hook.run()


def hook_check(ctx: hook.Context) -> bool:
    val = get_serviceIP(values=ctx.values)
    if val is None:
        return False
    return True

def get_serviceIP(values: dict):
    return module_values.get_value(path=f"{common.MODULE_NAME}.internal.dvcr.serviceIP", values=values)
 
def sans_generator(sans: list[str]) -> Callable[[hook.Context], list[str]]:
    def generator(ctx: hook.Context) -> list:
        sans.extend(["localhost", "127.0.0.1", get_serviceIP(ctx.values)])
        return sans
    return generator

if __name__ == "__main__":
    main()
