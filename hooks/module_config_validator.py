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

from typing import Callable
from deckhouse import hook
from lib.hooks.hook import Hook
from ipaddress import ip_network,ip_address,IPv4Network,IPv4Address
import common


class ModuleConfigValidateHook(Hook):
    KIND="ModuleConfig"
    API_VERSION="deckhouse.io/v1alpha1"
    SNAPSHOT_MODULE_CONFIG = "module-config"
    SNAPSHOT_NODES = "nodes"

    def __init__(self, module_name: str):
        self.module_name = module_name
        self.queue = f"/modules/{self.module_name}/{self.SNAPSHOT_MODULE_CONFIG}"
        self.path = "virtualization.internal.ready"

    def generate_config(self) -> dict:
        """executeHookOnEvent is empty because we need only execute at module start."""
        return {
            "configVersion": "v1",
            "kubernetes": [
                {
                    "name": self.SNAPSHOT_MODULE_CONFIG,
                    "executeHookOnSynchronization": True,
                    "executeHookOnEvent": [],
                    "apiVersion": self.API_VERSION,
                    "kind": self.KIND,
                    "nameSelector": {
                        "matchNames": [self.module_name]
                    },
                    "group": "main",
                    "jqFilter": '{"cidrs": .spec.settings.virtualMachineCIDRs}',
                    "queue": self.queue,
                    "keepFullObjectsInMemory": False
                },
                {
                    "name": self.SNAPSHOT_NODES,
                    "executeHookOnSynchronization": True,
                    "executeHookOnEvent": [],
                    "apiVersion": "v1",
                    "kind": "Node",
                    "group": "main",
                    "jqFilter": '{"addresses": .status.addresses}',
                    "queue": self.queue,
                    "keepFullObjectsInMemory": False
                }
            ]
        }

    def check_overlaps_cidrs(self, networks: list[IPv4Network]) -> None:
        """Check for overlapping CIDRs in a list of networks."""
        for i, net1 in enumerate(networks):
            for net2 in networks[i + 1:]:
                if net1.overlaps(net2):
                    raise ValueError(f"Overlapping CIDRs {net1} and {net2}")

    def check_node_addresses_overlap(self, networks: list[IPv4Network], node_addresses: list[IPv4Address]) -> None:
        """Check if node addresses overlap with any subnet."""
        for addr in node_addresses:
            for net in networks:
                if addr in net:
                    raise ValueError(f"Node address {addr} overlaps with subnet {net}")

    def reconcile(self) -> Callable[[hook.Context], None]:
        def r(ctx: hook.Context):
            cidrs: list[IPv4Network] = [
                ip_network(c)
                for c in ctx.snapshots.get(self.SNAPSHOT_MODULE_CONFIG, [{}])[0]
                .get("filterResult", {})
                .get("cidrs", [])
            ]

            try:
                self.check_overlaps_cidrs(cidrs)
            except ValueError as e:
                print(f"ERROR: {e}")

                self.set_value(self.path, ctx.values, False)

            node_addresses: list[IPv4Address] = [
                ip_address(addr["address"])
                for snap in ctx.snapshots.get(self.SNAPSHOT_NODES, [])
                for addr in (snap.get("filterResult", {}).get("addresses") or [])
                if addr.get("type") in {"InternalIP", "ExternalIP"}
            ]

            try:
                self.check_node_addresses_overlap(cidrs, node_addresses)
            except ValueError as e:
                print(f"ERROR: {e}")
                self.set_value(self.path, ctx.values, False)

            self.set_value(self.path, ctx.values, True)
        return r


if __name__ == "__main__":
    h = ModuleConfigValidateHook(common.MODULE_NAME)
    h.run()
