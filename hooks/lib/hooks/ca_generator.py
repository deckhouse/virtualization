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

from deckhouse import hook
from lib.certificate.certificate import CACertificateGenerator
from lib.certificate import parse
from datetime import timedelta
from typing import Callable
from lib.hooks.hook import Hook
import lib.utils as utils



class GenerateCAHook(Hook):
    """
    Сonfig for the hook that generates CA certificates.
    """
    SNAPSHOT_SECRETS_NAME = "secrets"
    SNAPSHOT_SECRETS_CHECK_NAME = "secretsCheck"

    def __init__(self, cn: str,
                 namespace: str,
                 ca_secret_name: str,
                 values_path_prefix: str,
                 expire: int = 31536000,
                 key_size: int = 4096,
                 algo: str = "rsa",
                 cert_outdated_duration: timedelta = timedelta(days=30),
                 module_name: str = None) -> None:
        super().__init__(module_name)
        self.cn = cn
        self.namespace = namespace
        self.ca_secret_name = ca_secret_name
        self.values_path_prefix = values_path_prefix
        self.expire = expire
        self.key_size = key_size
        self.algo = algo
        self.cert_outdated_duration = cert_outdated_duration
        self.queue = f"/modules/{self.module_name}/generate-ca"

        """
        :param cn: Certificate common Name. often it is module name
        :type cn: :py:class:`str`

        :param module_name: Module name
        :type module_name: :py:class:`str`

        :param namespace: Namespace for TLS secret.
        :type namespace: :py:class:`str`

        :param tls_secret_name: TLS secret name. 
        Secret must be TLS secret type https://kubernetes.io/docs/concepts/configuration/secret/#tls-secrets.
        CA certificate MUST set to ca.crt key.
        :type tls_secret_name: :py:class:`str`

        :param values_path_prefix: Prefix full path to store CA certificate TLS private key and cert.
	    full paths will be
	    values_path_prefix + .`crt` - TLS certificate
	    values_path_prefix + .`key` - TLS private key
	    Example: values_path_prefix =  'virtualization.internal.dvcrCert'
	    Data in values store as plain text
        :type values_path_prefix: :py:class:`str`

        :param expire: Optional. Validity period of SSL certificates.
        :type expire: :py:class:`int`

        :param key_size: Optional. Key Size.
        :type key_size: :py:class:`int`

        :param algo: Optional. Key generation algorithm. Supports only rsa and dsa.
        :type algo: :py:class:`str`

        :param cert_outdated_duration: Optional. (expire - cert_outdated_duration) is time to regenerate the certificate.
        :type cert_outdated_duration: :py:class:`timedelta`
        """

    def generate_config(self) -> dict:
        return {
            "configVersion": "v1",
            "beforeHelm": 1,
            "kubernetes": [
                {
                    "name": self.SNAPSHOT_SECRETS_NAME,
                    "apiVersion": "v1",
                    "kind": "Secret",
                    "nameSelector": {
                        "matchNames": [self.ca_secret_name]
                    },
                    "namespace": {
                        "nameSelector": {
                            "matchNames": [self.namespace]
                        }
                    },
                    "includeSnapshotsFrom": [self.SNAPSHOT_SECRETS_NAME],
                    "jqFilter": '{"data": .data}',
                    "queue": self.queue,
                    "keepFullObjectsInMemory": False
                },
            ],
            "schedule": [
                {
                    "name": self.SNAPSHOT_SECRETS_CHECK_NAME,
                    "crontab": "42 4 * * *"
                }
            ]
        }
    
    def reconcile(self) -> Callable[[hook.Context], None]:
        def r(ctx: hook.Context) -> None:
            if len(ctx.snapshots.get(self.SNAPSHOT_SECRETS_NAME, [])) == 0:
                print(f"Secret {self.ca_secret_name} not found. Generate new certififcates.")
                tls_data = self.generate_selfsigned_tls_data()
            else:
                data = ctx.snapshots[self.SNAPSHOT_SECRETS_NAME][0]["filterResult"]["data"]
                ca_outdated = self.is_outdated_ca(
                    utils.base64_decode(data.get("tls.crt", "")))
                if ca_outdated or data.get("tls.key", "") == "":
                    print(f"Certificates from secret {self.ca_secret_name} is invalid. Generate new certififcates.")
                    tls_data = self.generate_selfsigned_tls_data()
                else:
                    tls_data = {
                        "crt": data["tls.crt"],
                        "key": data["tls.key"]
                    }
            self.set_value(self.values_path_prefix, ctx.values, tls_data)
        return r
    
    def generate_selfsigned_tls_data(self) -> dict[str, str]:
        """
        Generate CA certificate. 
        Example: {
            "crt": "encoded in base64",
            "key": "encoded in base64"
        }
        :rtype: :py:class:`dict[str, str]`
        """
        ca = CACertificateGenerator(cn=self.cn,
                         expire=self.expire,
                         key_size=self.key_size,
                         algo=self.algo)
        tls_crt, tls_key = ca.generate()

        return {"crt": utils.base64_encode(tls_crt), "key": utils.base64_encode(tls_key)}
    
    def is_outdated_ca(self, ca: str) -> bool:
        """
        Issue a new certificate if there is no CA in the secret. Without CA it is not possible to validate the certificate.
        Check CA duration.
        :param ca: Raw CA
        :type ca: :py:class:`str`
        :rtype: :py:class:`bool`
        """
        return parse.is_outdated_ca(ca, self.cert_outdated_duration)