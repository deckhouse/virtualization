#!/usr/bin/env python3
#
# Copyright 2024 Flant JSC
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


from lib.hooks.internal_tls import *
from lib.module import values as module_values
from deckhouse.hook import Context
from typing import Callable
import common


def main():
    hook = GenerateCertificatesHook(
        CertitifacteRequest(
            cn=f"virtualization-controller",
            sansGenerator=default_sans([
                "virtualization-controller-admission-webhook",
                f"virtualization-controller-admission-webhook.{common.NAMESPACE}",
                f"virtualization-controller-admission-webhook.{common.NAMESPACE}.svc"],
            ),
            tls_secret_name="admission-webhook-secret",
            values_path_prefix=f"{common.MODULE_NAME}.internal.admissionWebhookCert"
        ),

        CertitifacteRequest(
            cn="dvcr",
            sansGenerator=dvcr_sans_generator([
                "dvcr",
                f"dvcr.{common.NAMESPACE}",
                f"dvcr.{common.NAMESPACE}.svc"]),
            tls_secret_name="dvcr-tls",
            values_path_prefix=f"{common.MODULE_NAME}.internal.dvcr.cert",
            before_gen_check=dvcr_before_check
        ),

        CertitifacteRequest(
            cn=f"virtualization-api",
            sansGenerator=default_sans([
                "virtualization-api",
                f"virtualization-api.{common.NAMESPACE}",
                f"virtualization-api.{common.NAMESPACE}.svc"],
            ),
            tls_secret_name="virtualization-api-tls",
            values_path_prefix=f"{common.MODULE_NAME}.internal.apiserver.cert"
        ),

        CertitifacteRequest(
            cn=f"virtualization-api-proxy",
            sansGenerator=empty_sans(),
            tls_secret_name="virtualization-api-proxy-tls",
            values_path_prefix=f"{common.MODULE_NAME}.internal.apiserver.proxyCert",
            extended_key_usages = [EXTENDED_KEY_USAGES[1]]
        ),

        namespace=common.NAMESPACE,
        module_name=common.MODULE_NAME,

        ca_request=CACertitifacteRequest(
            cn=f"virtualization.deckhouse.io",
            ca_secret_name="virtualization-ca",
            values_path_prefix=f"{common.MODULE_NAME}.internal.rootCA",
        ))

    hook.run()


def dvcr_before_check(ctx: Context) -> bool:
    val = dvcr_get_serviceIP(values=ctx.values)
    if val is None:
        return False
    return True


def dvcr_get_serviceIP(values: dict):
    return module_values.get_value(path=f"{common.MODULE_NAME}.internal.dvcr.serviceIP", values=values)


def dvcr_sans_generator(sans: list[str]) -> Callable[[Context], list[str]]:
    def generator(ctx: Context) -> list:
        sans.extend(["localhost", "127.0.0.1", dvcr_get_serviceIP(ctx.values)])
        return sans
    return generator


if __name__ == "__main__":
    main()
