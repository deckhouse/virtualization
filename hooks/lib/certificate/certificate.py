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

import random
import re
from OpenSSL import crypto
from ipaddress import ip_address


class Certificate:
    def __init__(self, cn: str, expire: int, key_size: int, algo: str) -> None:
        self.key = crypto.PKey()
        self.__with_key(algo=algo, size=key_size)
        self.cert = crypto.X509()
        self.cert.set_version(version=2)
        self.cert.get_subject().CN = cn
        self.cert.set_serial_number(random.getrandbits(64))
        self.cert.gmtime_adj_notBefore(0)
        self.cert.gmtime_adj_notAfter(expire)

    def get_subject(self) -> crypto.X509Name:
        return self.cert.get_subject()

    def __with_key(self, algo: str, size: int) -> None:
        if algo == "rsa":
            self.key.generate_key(crypto.TYPE_RSA, size)
        elif algo == "dsa":
            self.key.generate_key(crypto.TYPE_DSA, size)
        else:
            raise Exception(f"Algo {algo} is not support. Only [rsa, dsa]")

    def with_metadata(self, country: str = None,
                      state: str = None,
                      locality: str = None,
                      organisation_name: str = None,
                      organisational_unit_name: str = None):
        """
        Adds subjects to certificate.

        :param country: Optional. The country of the entity.
        :type country: :py:class:`str`

        :param state: Optional. The state or province of the entity.
        :type state: :py:class:`str`

        :param locality: Optional. The locality of the entity
        :type locality: :py:class:`str`

        :param organisation_name: Optional. The organization name of the entity.
        :type organisation_name: :py:class:`str`

        :param organisational_unit_name: Optional. The organizational unit of the entity.
        :type organisational_unit_name: :py:class:`str`
        """

        if country is not None:
            self.cert.get_subject().C = country
        if state is not None:
            self.cert.get_subject().ST = state
        if locality is not None:
            self.cert.get_subject().L = locality
        if organisation_name is not None:
            self.cert.get_subject().O = organisation_name
        if organisational_unit_name is not None:
            self.cert.get_subject().OU = organisational_unit_name
        return self

    def add_extension(self, type_name: str,
                      critical: bool,
                      value: str,
                      subject: crypto.X509 = None,
                      issuer: crypto.X509 = None):
        """
        Adds extensions to certificate.
        :param type_name: The name of the type of extension_ to create.
        :type type_name: :py:class:`str`

        :param critical: A flag indicating whether this is a critical
            extension.
        :type critical: :py:class:`bool`

        :param value: The OpenSSL textual representation of the extension's
            value.
        :type value: :py:class:`str`

        :param subject: Optional X509 certificate to use as subject.
        :type subject: :py:class:`crypto.X509`

        :param issuer: Optional X509 certificate to use as issuer.
        :type issuer: :py:class:`crypto.X509`
        """
        ext = crypto.X509Extension(type_name=str.encode(type_name),
                                   critical=critical,
                                   value=str.encode(value),
                                   subject=subject,
                                   issuer=issuer)
        self.cert.add_extensions(extensions=[ext])
        return self

    def generate(self) -> (bytes, bytes):
        """
        Generate certificate.
        :return: (certificate, key)
        :rtype: (:py:data:`bytes`, :py:data:`bytes`)
        """
        pub = crypto.dump_certificate(crypto.FILETYPE_PEM, self.cert)
        priv = crypto.dump_privatekey(crypto.FILETYPE_PEM, self.key)
        return pub, priv


class CACertificateGenerator(Certificate):
    """
    A class representing a generator CA certificate.
    """

    def __sign(self) -> None:
        self.cert.set_issuer(self.get_subject())
        self.cert.set_pubkey(self.key)
        self.cert.sign(self.key, 'sha256')

    def generate(self) -> (bytes, bytes):
        """
        Generate CA certificate.
        :return: (ca crt, ca key)
        :rtype: (:py:data:`bytes`, :py:data:`bytes`)
        """
        self.add_extension(type_name="subjectKeyIdentifier",
                           critical=False, value="hash", subject=self.cert)
        self.add_extension(type_name="authorityKeyIdentifier",
                           critical=False, value="keyid:always", issuer=self.cert)
        self.add_extension(type_name="basicConstraints",
                           critical=False, value="CA:TRUE")
        self.add_extension(type_name="keyUsage", critical=False,
                           value="keyCertSign, cRLSign, keyEncipherment")
        self.__sign()
        return super().generate()


class CertificateGenerator(Certificate):
    """
    A class representing a generator certificate.
    """

    def with_hosts(self, *hosts: str):
        """
        This function is used to add subject alternative names to a certificate. 
        It takes a variable number of hosts as parameters, and based on the type of host (IP or DNS).

        :param hosts: Variable number of hosts to be added as subject alternative names to the certificate.
        :type hosts: :py:class:`tuple`
        """
        alt_names = []
        for h in hosts:
            try:
                ip_address(h)
                alt_names.append(f"IP:{h}")
            except ValueError:
                if not is_valid_hostname(h):
                    continue
                alt_names.append(f"DNS:{h}")
        if len(alt_names) > 0:
            self.add_extension("subjectAltName", False, ", ".join(alt_names))
        return self

    def __sign(self, ca_subj: crypto.X509Name, ca_key: crypto.PKey) -> None:
        self.cert.set_issuer(ca_subj)
        self.cert.set_pubkey(self.key)
        self.cert.sign(ca_key, 'sha256')

    def generate(self, ca_subj: crypto.X509Name, ca_key: crypto.PKey) -> (bytes, bytes):
        """
        Generate certificate.
        :param ca_subj: CA subject.
        :type ca_subj: :py:class:`crypto.X509Name`
        :param ca_key: CA Key.
        :type ca_key: :py:class:`crypto.PKey`
        :return: (certificate, key)
        :rtype: (:py:data:`bytes`, :py:data:`bytes`)
        """
        self.__sign(ca_subj, ca_key)
        return super().generate()


def is_valid_hostname(hostname):
    if len(hostname) > 255:
        return False
    if hostname[-1] == ".":
        hostname = hostname[:-1]
    allowed = re.compile("(?!-)[A-Z\d-]{1,63}(?<!-)$", re.IGNORECASE)
    return all(allowed.match(x) for x in hostname.split("."))
