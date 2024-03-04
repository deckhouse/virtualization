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


from lib.hooks.internal_tls import GenerateCertificateHook, default_sans
import common

def main():
    hook = GenerateCertificateHook(
        cn=f"virtualization-api",
        sansGenerator=default_sans([
            "virtualization-api",
            f"virtualization-api.{common.NAMESPACE}",
            f"virtualization-api.{common.NAMESPACE}.svc"
        ]),
        namespace=common.NAMESPACE,
        tls_secret_name="virtualziation-api-tls",
        values_path_prefix=f"{common.MODULE_NAME}.internal.apiserver.cert")

    hook.run()


if __name__ == "__main__":
    main()
