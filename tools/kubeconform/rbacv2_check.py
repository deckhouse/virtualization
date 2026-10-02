#!/usr/bin/env python3

# Copyright 2026 Flant JSC
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

# Renders the chart for DKP versions on both sides of the RBACv2 model switch (1.78) and checks the
# label contract of the module ClusterRoles. The platform contract test in deckhouse/testing/rbacv2
# covers only the modules of the deckhouse repository.

import os
import re
import subprocess
import sys

import yaml

ROOT = os.path.abspath(os.path.join(os.path.dirname(__file__), "..", ".."))
VALUES = os.path.join(ROOT, "tools", "kubeconform", "fixtures", "module-values.yaml")
LEGACY_NAME = re.compile(r"^d8:(use|manage):")
AGGREGATE = re.compile(r"^rbac\.deckhouse\.io/aggregate-to-([a-z0-9-]+)-as$")
ANNOTATIONS = [f"{lang}.meta.deckhouse.io/{key}" for lang in ("en", "ru") for key in ("title", "description")]
LEGACY_COUNT = 22


def render(edition, version):
    out = subprocess.run(
        ["helm", "template", "virtualization", ROOT, "-f", VALUES,
         "--set", f"global.deckhouseEdition={edition}", "--set", f"global.deckhouseVersion={version}"],
        check=True, capture_output=True, text=True).stdout
    roles = [d for d in yaml.safe_load_all(out) if d and d.get("kind") == "ClusterRole"]
    return [r for r in roles if (r["metadata"].get("labels") or {}).get("rbac.deckhouse.io/kind")
            or (r["metadata"].get("labels") or {}).get("rbac.deckhouse.io/deprecated")]


def check_new(roles):
    errs, markers = [], {}
    for r in roles:
        name, labels = r["metadata"]["name"], r["metadata"].get("labels") or {}
        annotations = r["metadata"].get("annotations") or {}
        lineages = {m.group(1): v for k, v in labels.items() if (m := AGGREGATE.match(k))}
        kind = labels.get("rbac.deckhouse.io/kind")
        if labels.get("heritage") != "deckhouse" or labels.get("module") != "virtualization":
            errs.append(f"{name}: heritage and module labels are required")
        if missing := [a for a in ANNOTATIONS if a not in annotations]:
            errs.append(f"{name}: missing annotations {missing}")
        for selector in (r.get("aggregationRule") or {}).get("clusterRoleSelectors", []):
            match = selector.get("matchLabels", {})
            if "rbac.deckhouse.io/aggregate-to-virtualization-as" in match and "rbac.deckhouse.io/kind" not in match:
                errs.append(f"{name}: a selector on the virtualization lineage must pin rbac.deckhouse.io/kind, "
                            "or legacy use-capabilities of other modules leak into it")
            for lineage in ("cluster", "namespace"):
                if f"rbac.deckhouse.io/aggregate-to-{lineage}-as" in match and "module" not in match:
                    errs.append(f"{name}: a selector on the {lineage} lineage must pin the module label, "
                                "or it grants the rights of the whole lineage")
        if kind is None:
            if labels.get("rbac.deckhouse.io/deprecated") != "true" or not LEGACY_NAME.match(name):
                errs.append(f"{name}: an object without rbac.deckhouse.io/kind must be a legacy alias")
            if "rbac.deckhouse.io/deprecated-replaced-by" not in annotations:
                errs.append(f"{name}: an alias needs the rbac.deckhouse.io/deprecated-replaced-by annotation")
            if "rules" in r or lineages:
                errs.append(f"{name}: an alias has no rules key and no aggregation labels")
            # Every namespace is a project on 1.78: a new RoleBinding to a namespace alias without
            # the label is refused. Aliases of cluster-scoped roles stay non-delegatable.
            delegatable = name.startswith("d8:use:")
            if (labels.get("rbac.deckhouse.io/delegatable") == "true") != delegatable or \
                    (annotations.get("rbac.deckhouse.io/disabled-for-direct-use-in-projects") == "true") != delegatable:
                errs.append(f"{name}: a d8:use alias needs delegatable and disabled-for-direct-use-in-projects, "
                            "a d8:manage alias must not have them")
            continue
        if LEGACY_NAME.match(name) or "rbac.deckhouse.io/level" in labels:
            errs.append(f"{name}: legacy name or label in the new model")
        scope = labels.get("rbac.deckhouse.io/scope")
        # The d8:subsystem:cluster roles belong to deckhouse-authz: a second owner would fight it.
        if kind == "role":
            errs.append(f"{name}: the module renders no roles in the new model")
            continue
        if kind != "capability" or scope not in ("namespace", "system"):
            errs.append(f"{name}: unexpected kind {kind!r} or scope {scope!r}")
            continue
        action = name.rsplit(":", 1)[1]
        marker = labels.get("rbac.deckhouse.io/capability", "")
        if name != f"d8:{scope}-capability:virtualization:{action}" or marker != f"{scope}-capability.virtualization.{action}":
            errs.append(f"{name}: name or capability marker does not follow <scope>-capability.virtualization.<action>")
        if len(marker) > 63:
            errs.append(f"{name}: capability marker is {len(marker)} characters, a label value allows 63")
        markers.setdefault(marker, []).append(name)
        if not r.get("rules") or r.get("aggregationRule"):
            errs.append(f"{name}: a capability has rules and no aggregationRule")
        # A namespace capability that carries a subsystem lineage would be aggregated into a
        # cluster-wide role and grant its namespaced rules in every namespace.
        expected = {"namespace"} if scope == "namespace" else {"cluster"}
        if set(lineages) != expected:
            errs.append(f"{name}: aggregates into {sorted(lineages)}, expected {sorted(expected)}")
        if scope == "system" and labels.get("rbac.deckhouse.io/namespace") != "d8-virtualization":
            errs.append(f"{name}: a system capability needs rbac.deckhouse.io/namespace: d8-virtualization")
    errs += [f"capability marker {m} is not unique: {n}" for m, n in markers.items() if len(n) > 1]
    return errs


def check_legacy(roles):
    errs = [f"{r['metadata']['name']}: new model object in the legacy branch"
            for r in roles if not LEGACY_NAME.match(r["metadata"]["name"])]
    if len(roles) != LEGACY_COUNT:
        errs.append(f"legacy branch renders {len(roles)} objects, expected {LEGACY_COUNT}")
    return errs


def main():
    failed = False
    for edition in ("CE", "EE"):
        for version, new in (("v1.77.3", False), ("v1.78.0", True), ("dev", True)):
            roles = render(edition, version)
            errs = check_new(roles) if new else check_legacy(roles)
            print(f"rbacv2 {edition} {version}: {len(roles)} objects, {len(errs)} problems")
            for e in errs:
                print(f"  {e}")
            failed = failed or bool(errs)
    return 1 if failed else 0


if __name__ == "__main__":
    sys.exit(main())
