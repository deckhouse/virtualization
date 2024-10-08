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


class TLSData(dict):
    def __init__(self, *args, **kwargs):
        super().__init__(*args, **kwargs)

    def is_empty(self) -> bool:
        return len(self) == 0

    def set(self, key: str, value):
        if isinstance(value, str):
            if utils.is_base64(value):
                self[key] = value
                return self
            self[key] = utils.base64_encode_from_str(value)
            return self
        if isinstance(value, bytes):
            self[key] = utils.base64_encode(value)
            return self
        raise Exception(f"invalid type {type(value)}")

    def to_dict(self) -> dict:
        return dict(self)


class TLSSecretData(TLSData):
    def __init__(self, *args, **kwargs):
        super().__init__(*args, **kwargs)

    def CA(self) -> str:
        return self.get("ca.crt", "")

    def cert(self) -> str:
        return self.get("tls.crt", "")

    def key(self) -> str:
        return self.get("tls.key", "")

    def set_ca(self, ca):
        return self.set("ca.crt", ca)

    def set_cert(self, crt):
        return self.set("tls.crt", crt)

    def set_key(self, key):
        return self.set("tls.key", key)


class TLSValueData(TLSData):
    def __init__(self, *args, **kwargs):
        super().__init__(*args, **kwargs)

    def CA(self) -> str:
        return self.get("ca", "")

    def cert(self) -> str:
        return self.get("crt", "")

    def key(self) -> str:
        return self.get("key", "")

    def set_ca(self, ca):
        return self.set("ca", ca)

    def set_cert(self, crt):
        return self.set("crt", crt)

    def set_key(self, key):
        return self.set("key", key)


def convert_to_TLSValueData(data: TLSSecretData) -> TLSValueData:
    result = TLSValueData()
    return result.set_ca(data.CA()).set_cert(data.cert()).set_key(data.key())


def convert_to_TLSSecretData(data: TLSValueData) -> TLSSecretData:
    result = TLSSecretData()
    return result.set_ca(data.CA()).set_cert(data.cert()).set_key(data.key())


class CACertitifacteRequest:
    """
    Сonfig for the hook that generates CA certificate.
    """

    def __init__(self, cn: str,
                 ca_secret_name: str,
                 values_path_prefix: str,
                 expire: int = 31536000,
                 key_size: int = 4096,
                 cert_outdated_duration: timedelta = timedelta(days=30)) -> None:
        self.cn = cn
        self.ca_secret_name = ca_secret_name
        self.values_path_prefix = values_path_prefix
        self.expire = expire
        self.key_size = key_size
        self.cert_outdated_duration = cert_outdated_duration
        """
        :param cn: Certificate common Name. often it is module name
        :type cn: :py:class:`str`

        :param ca_secret_name: TLS secret name. 
        Secret must be TLS secret type https://kubernetes.io/docs/concepts/configuration/secret/#tls-secrets.
        CA certificate MUST set to ca.crt key.
        :type ca_secret_name: :py:class:`str`

        :param values_path_prefix: Prefix full path to store CA certificate TLS private key and cert.
	    full paths will be
	    values_path_prefix + .`crt` - TLS certificate
	    values_path_prefix + .`key` - TLS private key
	    Example: values_path_prefix =  'virtualization.internal.Cert'
	    Data in values store as plain text
        :type values_path_prefix: :py:class:`str`

        :param expire: Optional. Validity period of SSL certificates.
        :type expire: :py:class:`int`

        :param key_size: Optional. Key Size.
        :type key_size: :py:class:`int`

        :param cert_outdated_duration: Optional. (expire - cert_outdated_duration) is time to regenerate the certificate.
        :type cert_outdated_duration: :py:class:`timedelta`
        """


class CertitifacteRequest:
    """
    Сonfig for the hook that generates certificate.
    """

    def __init__(self, cn: str,
                 sansGenerator: Callable[[list[str]], Callable[[hook.Context], list[str]]],
                 tls_secret_name: str,
                 values_path_prefix: str,
                 key_usages: list[str] = [KEY_USAGES[2], KEY_USAGES[5]],
                 extended_key_usages: list[str] = [EXTENDED_KEY_USAGES[0]],
                 before_gen_check: Callable[[hook.Context], bool] = None,
                 expire: int = 31536000,
                 key_size: int = 4096,
                 cert_outdated_duration: timedelta = timedelta(days=30),
                 country: str = None,
                 state: str = None,
                 locality: str = None,
                 organisation_name: str = None,
                 organisational_unit_name: str = None) -> None:
        self.cn = cn
        self.sansGenerator = sansGenerator
        self.tls_secret_name = tls_secret_name
        self.values_path_prefix = values_path_prefix
        self.key_usages = key_usages
        self.extended_key_usages = extended_key_usages
        self.before_gen_check = before_gen_check
        self.expire = expire
        self.key_size = key_size
        self.cert_outdated_duration = cert_outdated_duration
        self.country = country
        self.state = state
        self.locality = locality
        self.organisation_name = organisation_name
        self.organisational_unit_name = organisational_unit_name
        """
        :param cn: Certificate common Name. often it is module name
        :type cn: :py:class:`str`

        :param sansGenerator: Function which returns list of domain to include into cert. Use default_sans
        :type sansGenerator: :py:class:`function`

        :param tls_secret_name: TLS secret name. 
        Secret must be TLS secret type https://kubernetes.io/docs/concepts/configuration/secret/#tls-secrets.
        CA certificate MUST set to ca.crt key.
        :type tls_secret_name: :py:class:`str`

        :param values_path_prefix: Prefix full path to store CA certificate TLS private key and cert.
	    full paths will be
	    values_path_prefix + .`ca`  - CA certificate
	    values_path_prefix + .`crt` - TLS certificate
	    values_path_prefix + .`key` - TLS private key
	    Example: values_path_prefix =  'virtualization.internal.Cert'
	    Data in values store as plain text
        :type values_path_prefix: :py:class:`str`

        :param key_usages: Optional. key_usages specifies valid usage contexts for keys.
        :type key_usages: :py:class:`list`

        :param extended_key_usages: Optional. extended_key_usages specifies valid usage contexts for keys.
        :type extended_key_usages: :py:class:`list`

        :param before_gen_check: Optional. Runs check function before hook execution. Function should return boolean 'continue' value
	    if return value is false - hook will stop its execution
	    if return value is true - hook will continue
        :type before_gen_check: :py:class:`function`

        :param expire: Optional. Validity period of SSL certificates.
        :type expire: :py:class:`int`

        :param key_size: Optional. Key Size.
        :type key_size: :py:class:`int`

        :param cert_outdated_duration: Optional. (expire - cert_outdated_duration) is time to regenerate the certificate.
        :type cert_outdated_duration: :py:class:`timedelta`
        """


class GenerateCertificatesHook(Hook):
    """
    Hook that generates certificates.
    """
    SNAPSHOT_SECRETS_NAME = "secrets"
    SNAPSHOT_SECRETS_CHECK_NAME = "secretsCheck"

    def __init__(self, *certificate_requests: CertitifacteRequest,
                 namespace: str,
                 module_name: str,
                 algo: str = "rsa",
                 ca_request: CACertitifacteRequest = None) -> None:
        self.module_name=module_name
        self.namespace = namespace
        self.algo = algo
        self.ca_request = ca_request
        self.queue = f"/modules/{self.module_name}/generate-certs"
        self.secrets_names = [i.tls_secret_name for i in certificate_requests]
        if ca_request is not None:
            self.secrets_names.append(ca_request.ca_secret_name)
        self.certificate_requests_map = {
            i.tls_secret_name: i for i in certificate_requests}

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
                        "matchNames": self.secrets_names
                    },
                    "namespace": {
                        "nameSelector": {
                            "matchNames": [self.namespace]
                        }
                    },
                    "includeSnapshotsFrom": [self.SNAPSHOT_SECRETS_NAME],
                    "jqFilter": '{"name": .metadata.name, "data": .data}',
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

    def reconcile(self) -> None:
        def r(ctx: hook.Context):
            snaps = {}
            for s in ctx.snapshots.get(self.SNAPSHOT_SECRETS_NAME, []):
                snaps[s["filterResult"]["name"]] = TLSSecretData(
                    s["filterResult"]["data"])
            ca_data = TLSSecretData()
            if self.__with_common_ca():
                tls_value_data = self.__sync_ca(self.ca_request,
                                                snaps.get(self.ca_request.ca_secret_name, TLSSecretData()))
                ca_data = convert_to_TLSSecretData(tls_value_data)
                self.set_value(self.ca_request.values_path_prefix,
                               ctx.values, tls_value_data.to_dict())

            for name, req in self.certificate_requests_map.items():
                if name == self.ca_request.ca_secret_name:
                    continue
                data = snaps.get(name, TLSSecretData())
                tls_value_data = self.__sync_cert(ctx, req, data, ca_data)
                if tls_value_data is None:
                    continue
                self.set_value(req.values_path_prefix,
                               ctx.values, tls_value_data.to_dict())
        return r

    def __with_common_ca(self) -> bool:
        return self.ca_request is not None

    def __sync_cert(self, ctx: hook.Context, req: CertitifacteRequest, data: TLSSecretData, ca_data: TLSSecretData) -> TLSValueData:
        if req.before_gen_check is not None:
            passed = req.before_gen_check(ctx)
            if not passed:
                return
        sans = req.sansGenerator(ctx)
        if data.is_empty():
            print(
                f"Secret {req.tls_secret_name} not found. Generate new certififcates.")
            if not ca_data.is_empty():
                return self.__generate_selfsigned_tls_data_with_ca(req=req, sans=sans, ca_data=ca_data)
            return self.__generate_selfsigned_tls_data(req=req, sans=sans)

        ca_not_equal = data.CA() == "" or data.CA() != ca_data.cert()
        ca_outdated = self.__is_outdated_ca(
            utils.base64_decode(data.CA()),
            req.cert_outdated_duration)
        cert_outdated = self.__is_irrelevant_cert(
            utils.base64_decode(data.cert()),
            sans,
            req.cert_outdated_duration)
        if ca_not_equal or ca_outdated or cert_outdated or data.key() == "":
            print(
                f"Certificates from secret {req.tls_secret_name} is invalid. Generate new certififcates.")
            if len(ca_data) > 0:
                return self.__generate_selfsigned_tls_data_with_ca(req=req, sans=sans, ca_data=ca_data)
            return self.__generate_selfsigned_tls_data(req=req, sans=sans)

        return convert_to_TLSValueData(data)

    def __sync_ca(self, req: CACertitifacteRequest, data: TLSSecretData) -> TLSValueData:
        regenerate = False
        if data.is_empty():
            print(
                f"Secret {req.ca_secret_name} not found. Generate new certififcates.")
            regenerate = True
        else:
            ca_outdated = self.__is_outdated_ca(
                utils.base64_decode(data.cert()),
                req.cert_outdated_duration)
            if ca_outdated or data.key() == "":
                print(
                    f"Certificates from secret {req.ca_secret_name} is invalid. Generate new certififcates.")
                regenerate = True
        if regenerate:
            generator = CACertificateGenerator(
                cn=req.cn, expire=req.expire, key_size=req.key_size, algo=self.algo)
            crt, key = generator.generate()
            d = TLSValueData()
            return d.set_cert(crt).set_key(key)

        return convert_to_TLSValueData(data)

    def __generate_selfsigned_tls_data_with_ca(self,
                                               req: CertitifacteRequest,
                                               sans: list,
                                               ca_data: TLSSecretData) -> TLSValueData:
        if ca_data.cert() == "" or ca_data.key() == "":
            raise Exception("rootCA not found")
        crt_x509 = parse.parse_certificate(
            crt=utils.base64_decode(ca_data.cert()))
        ca_subject = crt_x509.get_subject()
        ca_pkey = parse.parse_key(key=utils.base64_decode(ca_data.key()))

        cert = CertificateGenerator(cn=req.cn,
                                    expire=req.expire,
                                    key_size=req.key_size,
                                    algo=self.algo)
        if len(req.key_usages) > 0:
            key_usages = ", ".join(req.key_usages)
            cert.add_extension(type_name="keyUsage",
                               critical=False, value=key_usages)
        if len(req.extended_key_usages) > 0:
            extended_key_usages = ", ".join(req.extended_key_usages)
            cert.add_extension(type_name="extendedKeyUsage",
                               critical=False, value=extended_key_usages)
        crt, key = cert.with_metadata(country=req.country,
                                      state=req.state,
                                      locality=req.locality,
                                      organisation_name=req.organisation_name,
                                      organisational_unit_name=req.organisational_unit_name
                                      ).with_hosts(*sans).generate(ca_subj=ca_subject,
                                                                   ca_key=ca_pkey)
        result = TLSValueData()
        return result.set_ca(ca_data.cert()).set_cert(crt).set_key(key)

    def __generate_selfsigned_tls_data(self,
                                       req: CertitifacteRequest,
                                       sans: list) -> TLSValueData:
        ca = CACertificateGenerator(cn=f"CA {req.cn}",
                                    expire=req.expire,
                                    key_size=req.key_size,
                                    algo=self.algo)
        crt, key = ca.generate()
        ca_data = TLSSecretData()
        ca_data.set_cert(crt).set_key(key)
        return self.__generate_selfsigned_tls_data_with_ca(req, sans, ca_data)

    def __is_irrelevant_cert(self, crt_data: str, sans: list, cert_outdated_duration: timedelta) -> bool:
        return parse.is_irrelevant_cert(crt_data, sans, cert_outdated_duration)

    def __is_outdated_ca(self, ca: str, cert_outdated_duration: timedelta) -> bool:
        return parse.is_outdated_ca(ca, cert_outdated_duration)


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


def empty_sans() -> Callable[[hook.Context], list[str]]:
    def generator(ctx: hook.Context) -> list:
        return []
    return generator
