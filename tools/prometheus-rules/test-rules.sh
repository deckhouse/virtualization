#!/bin/bash

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

# Renders the chart, extracts the alerting rules from the rendered PrometheusRule
# resources and runs promtool against them: a syntax check plus the unit tests
# from ./tests, which assert that every alert fires when it should and stays
# silent when it should not.
#
# promtool is taken from the official Prometheus image, so nothing has to be
# installed on the host — the same approach as tools/kubeconform.

set -euo pipefail

if [[ ! -d ../../templates ]]; then
  echo "Error: run this script from tools/prometheus-rules"
  exit 1
fi

PROMETHEUS_IMAGE="${PROMETHEUS_IMAGE:-prom/prometheus:v3.1.0}"

if ! command -v helm >/dev/null; then
  echo "Error: Helm v3 is not installed or not in PATH"
  exit 1
fi

# Use a local promtool when it is available, fall back to the container image.
if command -v promtool >/dev/null; then
  function __promtool() { promtool "$@"; }
elif command -v docker >/dev/null; then
  function __promtool() {
    docker run --rm -i -v "$(pwd)":/workdir -w /workdir --entrypoint /bin/promtool "${PROMETHEUS_IMAGE}" "$@"
  }
else
  echo "Error: neither promtool nor docker is available"
  exit 1
fi

cleanup() { rm -f helm-render.yaml rendered-rules.yaml; }
trap cleanup EXIT

echo "==> Rendering the chart"
helm template virtualization ../.. -f ../kubeconform/fixtures/module-values.yaml --devel > helm-render.yaml

echo "==> Extracting alerting rules"
python3 - helm-render.yaml rendered-rules.yaml <<'PY'
import sys, yaml, io

src, dst = sys.argv[1], sys.argv[2]
groups = []
for doc in yaml.safe_load_all(io.open(src, encoding="utf-8")):
    if doc and doc.get("kind") == "PrometheusRule":
        groups.extend(doc["spec"]["groups"])
if not groups:
    print("Error: the chart rendered no PrometheusRule resources", file=sys.stderr)
    sys.exit(1)
io.open(dst, "w", encoding="utf-8").write(
    yaml.safe_dump({"groups": groups}, allow_unicode=True, sort_keys=False, width=10000))
print(f"    groups: {len(groups)}, rules: {sum(len(g['rules']) for g in groups)}")
PY

echo "==> promtool check rules"
__promtool check rules rendered-rules.yaml

echo "==> promtool test rules"
__promtool test rules tests/*.yaml
