#!/usr/bin/env python3
#
# Copyright 2025 Flant JSC
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

from ipaddress import IPv4Address, IPv4Network, ip_address, ip_network
from typing import Callable

import common
from deckhouse import hook
from lib.hooks.hook import Hook


class ModuleConfigValidateHook(Hook):
    KIND = "ModuleConfig"
    API_VERSION = "deckhouse.io/v1alpha1"
    SNAPSHOT_MODULE_CONFIG = "module-config"
    SNAPSHOT_NODES = "nodes"
    SNAPSHOT_STORAGE_PROFILES = "internalvirtualizationstorageprofiles"

    def __init__(self, module_name: str):
        self.module_name = module_name
        self.queue = f"/modules/{self.module_name}/{self.SNAPSHOT_MODULE_CONFIG}"

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
                    "nameSelector": {"matchNames": [self.module_name]},
                    "group": "main",
                    "jqFilter": '{"settings": .spec.settings}',
                    "queue": self.queue,
                    "keepFullObjectsInMemory": False,
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
                    "keepFullObjectsInMemory": False,
                },
                {
                    "name": self.SNAPSHOT_STORAGE_PROFILES,
                    "executeHookOnSynchronization": True,
                    "executeHookOnEvent": [],
                    "apiVersion": "cdi.internal.virtualization.deckhouse.io/v1beta1",
                    "kind": "InternalVirtualizationStorageProfile",
                    "group": "main",
                    "jqFilter": '{"profiles": .}',
                    "queue": self.queue,
                    "keepFullObjectsInMemory": False,
                },
            ],
        }

    def check_overlaps_cidrs(self, networks: list[IPv4Network]) -> None:
        """Check for overlapping CIDRs in a list of networks."""
        for i, net1 in enumerate(networks):
            for net2 in networks[i + 1 :]:
                if net1.overlaps(net2):
                    raise ValueError(f"Overlapping CIDRs {net1} and {net2}")

    def check_node_addresses_overlap(
        self, networks: list[IPv4Network], node_addresses: list[IPv4Address]
    ) -> None:
        """Check if node addresses overlap with any subnet."""
        for addr in node_addresses:
            for net in networks:
                if addr in net:
                    raise ValueError(f"Node address {addr} overlaps with subnet {net}")

    def validate_virtual_images_storage_class(self, vi_settings: dict) -> None:
        """Check that the StorageClass's `PersistentVolumeMode` is not the `Filesystem`."""
        for profile in vi_settings["storageProfiles"]:
            name = profile.get("metadata", dict()).get("name", "")
            if name != "" and name == vi_settings["defaultStorageClassName"]:
                claim_property_sets = profile.get("status", dict()).get(
                    "claimPropertySets", list()
                )
                try:
                    claim_property_set = claim_property_sets[0]
                    if claim_property_set["volumeMode"] == "Filesystem":
                        raise ValueError(
                            f"a `StorageClass` with the `PersistentVolumeFilesystem` mode cannot be used for `VirtualImages` currently: {name}"
                        )
                except (IndexError, KeyError):
                    raise ValueError(
                        f"failed to validate the `PersistentVolumeMode` of the `StorageProfile`: {name}"
                    )
        for profile in vi_settings["storageProfiles"]:
            name = profile.get("metadata", dict()).get("name", "")
            allowed_storage_classes = vi_settings["allowedStorageClassSelector"]["matchNames"]
            if name != "" and name in allowed_storage_classes:
                claim_property_sets = profile.get("status", dict()).get(
                    "claimPropertySets", list()
                )
                try:
                    claim_property_set = claim_property_sets[0]
                    if claim_property_set["volumeMode"] == "Filesystem":
                        raise ValueError(
                            f"a `StorageClass` with the `PersistentVolumeFilesystem` mode cannot be used for `VirtualImages` currently: {name}"
                        )
                except (IndexError, KeyError):
                    raise ValueError(
                        f"failed to validate the `PersistentVolumeMode` of the `StorageProfile`: {name}"
                    )

    def reconcile(self) -> Callable[[hook.Context], None]:
        def r(ctx: hook.Context):
            cidrs: list[IPv4Network] = [
                ip_network(c)
                for c in ctx.snapshots.get(self.SNAPSHOT_MODULE_CONFIG, [dict()])[0]
                .get("filterResult", dict())
                .get("settings", dict())
                .get("virtualMachineCIDRs", list())
            ]
            self.check_overlaps_cidrs(cidrs)

            node_addresses: list[IPv4Address] = [
                ip_address(addr["address"])
                for snap in ctx.snapshots.get(self.SNAPSHOT_NODES, [])
                for addr in (snap.get("filterResult", {}).get("addresses") or [])
                if addr.get("type") in {"InternalIP", "ExternalIP"}
            ]
            self.check_node_addresses_overlap(cidrs, node_addresses)

            vi_default_storage_class: str = (
                ctx.snapshots.get(self.SNAPSHOT_MODULE_CONFIG, [dict()])[0]
                .get("filterResult", dict())
                .get("settings", dict())
                .get("virtualImages", dict())
                .get("defaultStorageClassName", "")
            )
            storage_profiles: list[dict] = [
                profile.get("filterResult", dict()).get("profiles", dict())
                for profile in ctx.snapshots.get(self.SNAPSHOT_STORAGE_PROFILES, list())
            ]
            vi_allowedStorageClassSelector: list[str] = [
                sc
                for sc in ctx.snapshots.get(self.SNAPSHOT_MODULE_CONFIG, [dict()])[0]
                .get("filterResult", dict())
                .get("settings", dict())
                .get("virtualImages", dict())
                .get("allowedStorageClassSelector", dict())
                .get("matchNames", list())
            ]
            vi_settings: dict[str, any] = {
                "defaultStorageClassName": vi_default_storage_class,
                "allowedStorageClassSelector": {
                    "matchNames": vi_allowedStorageClassSelector
                },
                "storageProfiles": storage_profiles,
            }
            self.validate_virtual_images_storage_class(vi_settings)

        return r


if __name__ == "__main__":
    h = ModuleConfigValidateHook(common.MODULE_NAME)
    h.run()
