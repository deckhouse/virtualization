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

from lib.tests import testing
from lib.hooks.internal_tls import GenerateCertificatesHook, CertitifacteRequest, CACertitifacteRequest, default_sans
from lib.certificate import parse
import lib.utils as utils
from OpenSSL import crypto
from ipaddress import ip_address

NAME        = "test"
MODULE_NAME = NAME
NAMESPACE   = NAME
SANS = [
    NAME,
    f"{NAME}.{NAMESPACE}",
    f"{NAME}.{NAMESPACE}.svc"
]

hook_generate = GenerateCertificatesHook(
    CertitifacteRequest(
        cn=NAME,
        sansGenerator=default_sans(SANS),
        tls_secret_name=NAME,
        values_path_prefix=f"{MODULE_NAME}.internal.cert"),
    module_name=MODULE_NAME,
    namespace=NAMESPACE,
    ca_request=CACertitifacteRequest(
        cn=f"CA {NAME}",
        ca_secret_name="",
        values_path_prefix="")
)

hook_regenerate = GenerateCertificatesHook(
    CertitifacteRequest(
        cn=NAME,
        sansGenerator=default_sans(SANS),
        tls_secret_name=NAME,
        values_path_prefix=f"{MODULE_NAME}.internal.cert",
        expire=0),
    module_name=MODULE_NAME,
    namespace=NAMESPACE,
    ca_request=CACertitifacteRequest(
        cn=f"CA {NAME}",
        ca_secret_name="",
        values_path_prefix="")
)

binding_context = [
    {
        "binding": "binding",
        "snapshots": {}   
    }
]

values = {
    "global": {
        "modules": {
            "publicDomainTemplate": "example.com"
        },
        "discovery": {
             "clusterDomain": "cluster.local"
        }
    },
    MODULE_NAME: {
        "internal": {}
    }
}

class TestCertificate(testing.TestHook):
    secret_data = {}
    sans_default = SANS + ["localhost", "127.0.0.1"]

    @staticmethod
    def parse_certificate(crt: str) -> crypto.X509:
        return parse.parse_certificate(utils.base64_decode(crt))

    def check_data(self):
        self.assertGreater(len(self.values[MODULE_NAME]["internal"].get("cert", {})), 0)
        self.secret_data = self.values[MODULE_NAME]["internal"]["cert"]
        self.assertTrue(utils.is_base64(self.secret_data.get("ca", "")))
        self.assertTrue(utils.is_base64(self.secret_data.get("crt", "")))
        self.assertTrue(utils.is_base64(self.secret_data.get("key", "")))

    def check_sans(self, crt: crypto.X509) -> bool:
        sans_from_cert = parse.get_certificate_san(crt)
        sans = []
        for san in self.sans_default:
            try:
                ip_address(san)
                sans.append(f"IP Address:{san}")
            except ValueError:
                sans.append(f"DNS:{san}")
        sans_from_cert.sort()
        sans.sort()
        self.assertEqual(sans_from_cert, sans)
        
    def verify_certificate(self, ca: crypto.X509, crt: crypto.X509) -> crypto.X509StoreContextError:
        store = crypto.X509Store()
        store.add_cert(ca)
        ctx = crypto.X509StoreContext(store, crt)
        try:
            ctx.verify_certificate()
            return None
        except crypto.X509StoreContextError as e:
            return e

class TestGenerateCertificate(TestCertificate):
    def setUp(self):
        self.func            = hook_generate.reconcile()
        self.bindind_context = binding_context
        self.values          = values
    def test_generate_certificate(self):
        self.hook_run()
        self.check_data()
        ca = self.parse_certificate(self.secret_data["ca"])
        crt = self.parse_certificate(self.secret_data["crt"])
        if (e := self.verify_certificate(ca, crt)) is not None:
            self.fail(f"Certificate is not verify. Raised an exception: {e} ")
        self.check_sans(crt)

class TestReGenerateCertificate(TestCertificate):
    def setUp(self):
        self.func            = hook_regenerate.reconcile()
        self.bindind_context = binding_context
        self.values          = values
        self.hook_run()
        self.bindind_context[0]["snapshots"] = {
            hook_regenerate.SNAPSHOT_SECRETS_NAME : [
                {
                    "filterResult": {
                        "name": NAME,
                        "data": {
                            "ca.crt" : self.values[MODULE_NAME]["internal"]["cert"]["ca"], 
                            "tls.crt": self.values[MODULE_NAME]["internal"]["cert"]["crt"],
                            "key.crt": self.values[MODULE_NAME]["internal"]["cert"]["key"]
                            }
                        }
                    }
                ]
            }
        self.func            = hook_generate.reconcile()

    def test_regenerate_certificate(self):
        self.check_data()
        ca = self.parse_certificate(self.secret_data["ca"])
        crt = self.parse_certificate(self.secret_data["crt"])
        if self.verify_certificate(ca, crt) is None:
            self.fail(f"Certificate has not expired")
        self.hook_run()
        self.check_data()
        ca = self.parse_certificate(self.secret_data["ca"])
        crt = self.parse_certificate(self.secret_data["crt"])
        if (e := self.verify_certificate(ca, crt)) is not None:
            self.fail(f"Certificate is not verify. Raised an exception: {e} ")
        self.check_sans(crt)
