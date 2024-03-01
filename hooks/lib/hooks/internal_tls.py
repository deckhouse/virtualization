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
from lib.certificate.certificate import CACertificateGenerator, CertificateGenerator
from lib.certificate import parse
from datetime import timedelta
from typing import Callable
from lib.hooks.hook import Hook
from lib.hooks import common
import lib.utils as utils

KEY_USAGES = {
    0: "digitalSignature",
    1: "nonRepudiation",
    2: "keyEncipherment",
    3: "dataEncipherment",
    4: "keyAgreement",
    5: "keyCertSign",
    6: "cRLSign",
    7: "encipherOnly",
    8: "decipherOnly"
}

EXTENDED_KEY_USAGES = {
    0: "serverAuth",
    1: "clientAuth",
    2: "codeSigning",
    3: "emailProtection",
    4: "OCSPSigning"
}


class GenerateCertificateHook(Hook):
    """
    Сonfig for the hook that generates certificates.
    """
    SNAPSHOT_SECRETS_NAME = "secrets"
    SNAPSHOT_SECRETS_CHECK_NAME = "secretsCheck"

    def __init__(self, cn: str,
                 sansGenerator: Callable[[list[str]], Callable[[hook.Context], list[str]]],
                 namespace: str,
                 tls_secret_name: str,
                 values_path_prefix: str,
                 module_name: str = None,
                 key_usages: list[str] = [KEY_USAGES[2], KEY_USAGES[5]],
                 extended_key_usages: list[str] = [EXTENDED_KEY_USAGES[0]],
                 before_hook_check: Callable[[hook.Context], bool] = None,
                 expire: int = 31536000,
                 key_size: int = 4096,
                 algo: str = "rsa",
                 cert_outdated_duration: timedelta = timedelta(days=30),
                 country: str = None,
                 state: str = None,
                 locality: str = None,
                 organisation_name: str = None,
                 organisational_unit_name: str = None) -> None:
        super().__init__(module_name=module_name)
        self.cn = cn
        self.sansGenerator = sansGenerator
        self.namespace = namespace
        self.tls_secret_name = tls_secret_name
        self.values_path_prefix = values_path_prefix
        self.key_usages = key_usages
        self.extended_key_usages = extended_key_usages
        self.before_hook_check = before_hook_check
        self.expire = expire
        self.key_size = key_size
        self.algo = algo
        self.cert_outdated_duration = cert_outdated_duration
        self.country = country
        self.state = state
        self.locality = locality
        self.organisation_name = organisation_name
        self.organisational_unit_name = organisational_unit_name
        self.queue = f"/modules/{self.module_name}/generate-certs"
        """
        :param module_name: Module name
        :type module_name: :py:class:`str`

        :param cn: Certificate common Name. often it is module name
        :type cn: :py:class:`str`

        :param sansGenerator: Function which returns list of domain to include into cert. Use default_sans
        :type sansGenerator: :py:class:`function`

        :param namespace: Namespace for TLS secret.
        :type namespace: :py:class:`str`

        :param tls_secret_name: TLS secret name. 
        Secret must be TLS secret type https://kubernetes.io/docs/concepts/configuration/secret/#tls-secrets.
        CA certificate MUST set to ca.crt key.
        :type tls_secret_name: :py:class:`str`

        :param values_path_prefix: Prefix full path to store CA certificate TLS private key and cert.
	    full paths will be
	    values_path_prefix + .`ca`  - CA certificate
	    values_path_prefix + .`crt` - TLS certificate
	    values_path_prefix + .`key` - TLS private key
	    Example: values_path_prefix =  'virtualization.internal.dvcrCert'
	    Data in values store as plain text
        :type values_path_prefix: :py:class:`str`

        :param key_usages: Optional. key_usages specifies valid usage contexts for keys.
        :type key_usages: :py:class:`list`

        :param extended_key_usages: Optional. extended_key_usages specifies valid usage contexts for keys.
        :type extended_key_usages: :py:class:`list`

        :param before_hook_check: Optional. Runs check function before hook execution. Function should return boolean 'continue' value
	    if return value is false - hook will stop its execution
	    if return value is true - hook will continue
        :type before_hook_check: :py:class:`function`

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
            "beforeHelm": 5,
            "kubernetes": [
                {
                    "name": self.SNAPSHOT_SECRETS_NAME,
                    "apiVersion": "v1",
                    "kind": "Secret",
                    "nameSelector": {
                        "matchNames": [self.tls_secret_name]
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
            tls_data = {}
            if self.before_hook_check is not None:
                passed = self.before_hook_check(ctx)
                if not passed:
                    return
            sans = self.sansGenerator(ctx)
            if len(ctx.snapshots.get(self.SNAPSHOT_SECRETS_NAME, [])) == 0:
                print(f"Secret {self.tls_secret_name} not found. Generate new certififcates.")
                tls_data = self.generate_selfsigned_tls_data(sans=sans)
            else:
                data = ctx.snapshots[self.SNAPSHOT_SECRETS_NAME][0]["filterResult"]["data"]
                ca_outdated = self.is_outdated_ca(
                    utils.base64_decode(data.get("ca.crt", "")))
                cert_outdated = self.is_irrelevant_cert(
                    utils.base64_decode(data.get("tls.crt", "")), sans)
                if ca_outdated or cert_outdated or data.get("tls.key", "") == "":
                    print(f"Certificates from secret {self.tls_secret_name} is invalid. Generate new certififcates.")
                    tls_data = self.generate_selfsigned_tls_data(sans=sans)
                else:
                    tls_data = {
                        "ca": data["ca.crt"],
                        "crt": data["tls.crt"],
                        "key": data["tls.key"]
                    }
            self.set_value(self.values_path_prefix, ctx.values, tls_data)
        return r

    def generate_selfsigned_tls_data(self, sans: list) -> dict[str, str]:
        """
        Generate self signed certificate. 
        :param sans: List of sans.
        :type sans: :py:class:`list`
        Example: {
            "ca": "encoded in base64",
            "crt": "encoded in base64",
            "key": "encoded in base64"
        }
        :rtype: :py:class:`dict[str, str]`
        """
        ca = CACertificateGenerator(cn=f"CA {self.cn}",
                         expire=self.expire,
                         key_size=self.key_size,
                         algo=self.algo)
        ca_crt, _ = ca.generate()
        cert = CertificateGenerator(cn=self.cn,
                                    expire=self.expire,
                                    key_size=self.key_size,
                                    algo=self.algo)
        if len(self.key_usages) > 0:
            key_usages = ", ".join(self.key_usages)
            cert.add_extension(type_name="keyUsage",
                               critical=False, value=key_usages)
        if len(self.extended_key_usages) > 0:
            extended_key_usages = ", ".join(self.extended_key_usages)
            cert.add_extension(type_name="extendedKeyUsage",
                               critical=False, value=extended_key_usages)
        crt, key = cert.with_metadata(country=self.country,
                                      state=self.state,
                                      locality=self.locality,
                                      organisation_name=self.organisation_name,
                                      organisational_unit_name=self.organisational_unit_name
                                      ).with_hosts(*sans).generate(ca_subj=ca.get_subject(),
                                                                   ca_key=ca.key)
        return {"ca": utils.base64_encode(ca_crt),
                "crt": utils.base64_encode(crt),
                "key": utils.base64_encode(key)}

    def is_irrelevant_cert(self, crt_data: str, sans: list) -> bool:
        """
        Check certificate duration and SANs list
        :param crt_data: Raw certificate
        :type crt_data: :py:class:`str`
        :param sans: List of sans.
        :type sans: :py:class:`list`
        :rtype: :py:class:`bool`
        """
        return parse.is_irrelevant_cert(crt_data, sans, self.cert_outdated_duration)

    def is_outdated_ca(self, ca: str) -> bool:
        """
        Issue a new certificate if there is no CA in the secret. Without CA it is not possible to validate the certificate.
        Check CA duration.
        :param ca: Raw CA
        :type ca: :py:class:`str`
        :rtype: :py:class:`bool`
        """
        return parse.is_outdated_ca(ca, self.cert_outdated_duration)

def default_sans(sans: list[str]) -> Callable[[hook.Context], list[str]]:
    """
    Generate list of sans for certificate
    :param sans: List of alt names.
    :type sans: :py:class:`list[str]`
    cluster_domain_san(san) to generate sans with respect of cluster domain (e.g.: "app.default.svc" with "cluster.local" value will give: app.default.svc.cluster.local

    public_domain_san(san)
    """
    def generate_sans(ctx: hook.Context) -> list[str]:
        res = ["localhost", "127.0.0.1"]
        public_domain = str(ctx.values["global"]["modules"].get(
            "publicDomainTemplate", ""))
        cluster_domain = str(
            ctx.values["global"]["discovery"].get("clusterDomain", ""))
        for san in sans:
            san.startswith(common.PUBLIC_DOMAIN_PREFIX)
            if san.startswith(common.PUBLIC_DOMAIN_PREFIX) and public_domain != "":
                san = common.get_public_domain_san(san, public_domain)
            elif san.startswith(common.CLUSTER_DOMAIN_PREFIX) and cluster_domain != "":
                san = common.get_cluster_domain_san(san, cluster_domain)
            res.append(san)
        return res
    return generate_sans
