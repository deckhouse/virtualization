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

from OpenSSL import crypto
from datetime import datetime, timedelta
from ipaddress import ip_address

def parse_certificate(crt: str) -> crypto.X509:
    return crypto.load_certificate(crypto.FILETYPE_PEM, crt)

def parse_key(key: str) -> crypto.PKey:
    return crypto.load_privatekey(crypto.FILETYPE_PEM, key)

def get_certificate_san(crt: crypto.X509) -> list[str]:
    san = ''
    ext_count = crt.get_extension_count()
    for i in range(0, ext_count):
        ext = crt.get_extension(i)
        if 'subjectAltName' in str(ext.get_short_name()):
            san = ext.__str__()
    return san.split(', ')

def is_outdated_ca(ca: str, cert_outdated_duration: timedelta) -> bool:
    """
    Issue a new certificate if there is no CA in the secret. Without CA it is not possible to validate the certificate.
    Check CA duration.
    :param ca: Raw CA
    :type ca: :py:class:`str`
    :param cert_outdated_duration: Optional. (expire - cert_outdated_duration) is time to regenerate the certificate.
    :type cert_outdated_duration: :py:class:`timedelta`
    :rtype: :py:class:`bool`
    """
    if len(ca) == 0:
        return True
    crt = parse_certificate(ca)
    return cert_renew_deadline_exceeded(crt, cert_outdated_duration)

def cert_renew_deadline_exceeded(crt: crypto.X509, cert_outdated_duration: timedelta) -> bool:
    """
    Check certificate 
    :param crt: Certificate
    :type crt: :py:class:`crypto.X509`
    :param cert_outdated_duration: Optional. (expire - cert_outdated_duration) is time to regenerate the certificate.
    :type cert_outdated_duration: :py:class:`timedelta`
    :return: 
        if timeNow > expire - cert_outdated_duration:
            return True
        return False
    :rtype: :py:class:`bool`
    """
    not_after = datetime.strptime(
        crt.get_notAfter().decode('ascii'), '%Y%m%d%H%M%SZ')
    if datetime.now() > not_after - cert_outdated_duration:
        return True
    return False

def is_irrelevant_cert(crt_data: str, sans: list, cert_outdated_duration: timedelta) -> bool:
    """
    Check certificate duration and SANs list
    :param crt_data: Raw certificate
    :type crt_data: :py:class:`str`
    :param sans: List of sans.
    :type sans: :py:class:`list`
    :param cert_outdated_duration: Optional. (expire - cert_outdated_duration) is time to regenerate the certificate.
    :type cert_outdated_duration: :py:class:`timedelta`
    :rtype: :py:class:`bool`
    """
    if len(crt_data) == 0:
        return True
    crt = parse_certificate(crt_data)
    if cert_renew_deadline_exceeded(crt, cert_outdated_duration):
        return True
    alt_names = []
    for san in sans:
        try:
            ip_address(san)
            alt_names.append(f"IP Address:{san}")
        except ValueError:
            alt_names.append(f"DNS:{san}")
    cert_sans = get_certificate_san(crt)
    cert_sans.sort()
    alt_names.sort()
    if cert_sans != alt_names:
        return True
    return False