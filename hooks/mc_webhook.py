#!/usr/bin/env python3
#
# Copyright 2024 Flant JSC
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

from typing import Callable
from deckhouse import hook
from lib.hooks.hook import Hook
from ipaddress import ip_network,ip_address
import common

def parse_ip_address(ip_string):
    return ip_string.replace("ip-", "").replace("-", ".")

class ModuleConfigValidateHook(Hook):
    SNAPSHOT_NAME = "virtualmachineipaddresslease"
    VALIDATOR_NAME = "moduleconfig.virtualization.settings"

    def __init__(self, module_name: str):
        self.module_name = module_name
        self.namespace = common.NAMESPACE

    def generate_config(self) -> dict:
        return {
            "configVersion": "v1",
            "kubernetes": [
                {
                    "name": self.SNAPSHOT_NAME,
                    "apiVersion": "v1alpha2",
                    "kind": "VirtualMachineIPAddressLease",
                    "group": "main",
                    "jqFilter": '{"name": .metadata.name}',
                    "queue": f"/modules/{self.module_name}/mc-webhook",
                    "keepFullObjectsInMemory": False
                },
            ],
            "kubernetesValidating": [
                {
                    "name": self.VALIDATOR_NAME,
                    "group": "main",
                    "rules": [
                        {
                            "apiVersions": ["v1alpha1"],
                            "apiGroups": ["deckhouse.io"],
                            "resources": ["moduleconfigs"],
                            "operations": ["UPDATE"],
                            "scope": "Cluster",
                        }
                    ],
                    "failurePolicy": "Fail",
                    "timeoutSeconds": 30,
                }
            ]
        }

    @staticmethod
    def __allow(ctx: hook.Context, msg: str):
        ctx.output.validations.allow(msg)

    @staticmethod
    def __deny(ctx: hook.Context, msg: str):
        ctx.output.validations.deny(msg)

    def reconcile(self) -> Callable[[hook.Context], None]:
        def r(ctx: hook.Context):

            request = ctx.binding_context.get("review", {}).get("request")
            if len(request) == 0:
                self.__allow(ctx, "")
                return

            kind = request.get("kind", {}).get("kind", "")
            name = request.get("name", "")
            if kind != "ModuleConfig" or name != self.module_name:
                self.__allow(ctx, "")
                return

            lease_names = [n["filterResult"]["name"] for n in ctx.snapshots.get(self.SNAPSHOT_NAME, [])]
            if len(lease_names) == 0:
                self.__allow(ctx, "")
                return

            old_subnetes = request.get("oldObject", {}).get("spec", {}).get("settings", {}).get("virtualMachineCIDRs")
            new_subnets = request.get("object", {}).get("spec", {}).get("settings", {}).get("virtualMachineCIDRs")

            validate_subnets = []

            if len(new_subnets) != len(old_subnetes):
                validate_subnets = [ip_network(s) for s in old_subnetes if s not in new_subnets]

            if len(validate_subnets) == 0:
                self.__allow(ctx, "")
                return

            for name in lease_names:
                ip = ip_address(parse_ip_address(name))
                for subnet in validate_subnets:
                    if ip in subnet:
                        self.__deny(ctx, f"Subnet {subnet} is in use by one or more IP addresses.")
                        return

            self.__allow(ctx, "")
        return r


if __name__ == "__main__":
    h = ModuleConfigValidateHook(common.MODULE_NAME)
    h.run()
