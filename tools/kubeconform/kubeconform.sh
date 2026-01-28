#!/bin/bash

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

if [[ ! -d ../../templates ]]; then
  echo "Error: Run this script from tools/kubeconform"
  exit 1
fi

# Check helm.
if ! which helm 2>&1 >/dev/null ; then
  echo "Error: Helm v3 is not installed or not in PATH"
  exit 1
fi

# Check jq.
if ! which jq 2>&1 >/dev/null ; then
  echo "Error: jq is not installed or not in PATH"
  exit 1
fi

# Check kubeconform
KUBECONFORM_IMAGE=ghcr.io/yannh/kubeconform:latest
USE_DOCKER=false

function __kubeconform() {
  if [[ $USE_DOCKER == true ]]; then
    docker run --rm -i -v $(pwd):/workdir -w /workdir --entrypoint /kubeconform $KUBECONFORM_IMAGE "$@"
  else
    kubeconform "$@"
  fi
}

if which kubeconform 2>&1 >/dev/null ; then
  echo Use local kubeconform version: $(kubeconform -v) >&2
elif which docker 2>&1 >/dev/null ; then
  echo Use kubeconform via docker run >&2
  USE_DOCKER=true
elif [ "$(uname)" = "Darwin" ]; then
  echo "Please, install either Docker Desktop, or kubeconform with 'brew install kubeconform' and run the script again."
  exit 1
else
  echo "Please, install either docker, or kubeconform binary from releases page https://github.com/yannh/kubeconform/releases and run the script again."
  exit 1
fi

if [[ ! -d kubeconform.git ]]; then
  echo Clone kubeconform repository to transform schemas ...
  git clone https://github.com/yannh/kubeconform.git kubeconform.git
fi

if [[ ! -d schemas ]]; then
  mkdir -p schemas
  cd  schemas

  echo Download Deckhouse CRDs ...
  echo "  VerticalPodAutoscaler"
  curl -LOs https://raw.githubusercontent.com/deckhouse/deckhouse/main/modules/302-vertical-pod-autoscaler/crds/verticalpodautoscaler.yaml
  echo "  ScrapeConfig"
  curl -LOs https://raw.githubusercontent.com/deckhouse/deckhouse/main/modules/200-operator-prometheus/crds/scrapeconfigs.yaml
  echo "  ServiceMonitor"
  curl -LOs https://raw.githubusercontent.com/deckhouse/deckhouse/main/modules/200-operator-prometheus/crds/servicemonitors.yaml
  echo "  PrometheusRule"
  curl -LOs https://raw.githubusercontent.com/deckhouse/deckhouse/main/modules/200-operator-prometheus/crds/internal/prometheusrules.yaml
  echo "  NodeGroupConfiguration"
  curl -LOs https://raw.githubusercontent.com/deckhouse/deckhouse/main/modules/040-node-manager/crds/nodegroupconfiguration.yaml
  echo "  Certificate"
  curl -LOs https://raw.githubusercontent.com/deckhouse/deckhouse/main/modules/101-cert-manager/crds/cert-manager/cert-manager.io_certificates.yaml
  # curl -LOs https://raw.githubusercontent.com/deckhouse/deckhouse/main/modules/101-cert-manager/crds/crd-certificates.yaml
  echo "  GrafanaDashboardDefinition"
  curl -LOs https://raw.githubusercontent.com/deckhouse/deckhouse/main/modules/300-prometheus/crds/grafanadashboarddefinition.yaml
  echo " ClusterLoggingConfig"
  curl -LOs https://raw.githubusercontent.com/deckhouse/deckhouse/main/modules/460-log-shipper/crds/cluster-logging-config.yaml
  echo " ClusterLogDestination"
  curl -LOs https://raw.githubusercontent.com/deckhouse/deckhouse/main/modules/460-log-shipper/crds/cluster-log-destination.yaml
  echo " Descheduler"
  curl -LOs https://raw.githubusercontent.com/deckhouse/deckhouse/main/modules/400-descheduler/crds/deschedulers.yaml

  # Transform CRDs to JSON schemas.
  export FILENAME_FORMAT='{kind}-{group}-{version}'
  echo Transform Deckhouse CRDs ...
  for crdYaml in *.yaml ; do
    ../kubeconform.git/scripts/openapi2jsonschema.py $crdYaml
  done
  echo Transform Embedded Kubevirt and CDI CRDs ...
  for crdYaml in ../../../crds/embedded/*.yaml ; do
    ../kubeconform.git/scripts/openapi2jsonschema.py $crdYaml
  done

  echo Transform Virtualization module CRDs ...
  for crdYaml in $( ls -1 ../../../crds/*.yaml | grep -v doc-ru- ) ; do
    ../kubeconform.git/scripts/openapi2jsonschema.py $crdYaml
  done

  # Transform Descheduler CRD schemas
  # If the CRD's metadata defines specific properties, kubeconform will enforce validation.
  # Any field present in the Custom Resource (CR) that isn’t defined in the schema—such as metadata.labels—will cause validation to fail.
  find -iname "descheduler-deckhouse-*.json" | while read f ; do jq '(.properties.metadata) |= {type: "object"}' "$f" > tmp.json && mv tmp.json "$f" ; done

  cd ..
fi

HELM_RENDER=helm-template-render.yaml

helm template virtualization ../.. -f fixtures/module-values.yaml --debug --devel > helm-template-render.yaml

exitCode=$?
if [[ $exitCode -ne 0 ]]; then
  echo "Error: Helm template failed with exit code $exitCode"
  exit $exitCode
fi

cat ${HELM_RENDER} | __kubeconform -verbose -strict \
   -kubernetes-version 1.30.0 \
   -schema-location default \
   -schema-location 'schemas/{{ .ResourceKind }}{{ .KindSuffix }}.json' \
   -output json - > kubeconform-report.json

if ! jq type kubeconform-report.json 2>&1 >/dev/null ; then
  echo "Error: kubeconform-report.json is not a valid JSON file"
  cat kubeconform-report.json
  exit 1
fi

# Pretty print report in this form:
#
# VALID     VirtualMachineClass host-passthrough
# VALID     VirtualMachineClass host
# VALID     VirtualMachineClass generic
# ERROR     ServiceMonitor virtualization-virt-handler
#   error unmarshalling resource: error converting YAML to JSON: yaml: unmarshal errors:
#     line 235: key "namespaceSelector" already set in map
#     line 238: key "selector" already set in map
#     line 19: key "spec" already set in map
# ERROR     Job virtualization-pre-delete-hook
#   error unmarshalling resource: error converting YAML to JSON: yaml: unmarshal errors:
#     line 21: key "labels" already set in map
# INVALID   Service virtualization-controller-metrics
#   - /spec/ports/0:
#       additionalProperties 'type', 'clusterIP' not allowed
# ---
# Summary:
#   - valid: 97
#   - no schema: 0
#   - errors: 2
#   - skipped: 0
#   - invalid: 1

cat kubeconform-report.json | jq -r '
def indent(n): split("\n")|map((" "*n )+.)|join("\n") ;
def noSchemaError(msg): msg|test("could not find schema for") ;
def regularError(msg): msg|test("could not find schema for")|not ;

(.resources| sort_by(.kind, .name)) as $r |

 [$r[]|select(.status == "statusValid")] as $valid |
 [$r[]|select(.status == "statusSkipped")] as $skipped |
 [$r[]|select(.status == "statusError" and noSchemaError(.msg) )] as $noSchema |
 [$r[]|select(.status == "statusError" and regularError(.msg) )] as $errors |
 [$r[]|select(.status == "statusInvalid")] as $invalid |

( $valid    | map("VALID     " + .kind + " " + .name)) as $validResources |

( $skipped  | map("SKIPPED   " + .kind + " " + .name)) as $skippedResources |

( $noSchema | map("NO SCHEMA " + .kind + " " + .name)) as $noSchemaResources |

( $errors  | map(
                  "ERROR     " + .kind + " " + .name,
                 (.msg | indent(2))
)) as $errorResources |

( $invalid  | map("INVALID   " + .kind + " " + .name ,
    (.validationErrors |
              map(
                 ("- " + .path + ":" | indent(2)),
                 (.msg | indent(6))
              )
    )
)) as $invalidResources |

[
  "--- Kubeconform report ---",
  $validResources,
  $skippedResources,
  $noSchemaResources,
  $errorResources,
  $invalidResources,
  "------- Summary -------",
  "      valid: " + ($valid|length|tostring),
  "    skipped: " + ($skipped|length|tostring),
  "  no schema: " + ($noSchema|length|tostring),
  "     errors: " + ($errors|length|tostring),
  "    invalid: " + ($invalid|length|tostring)
] | flatten | join("\n")'

exitCode=$(jq -r '[.resources[] | select(.status == "statusError" or .status == "statusInvalid")] | length' kubeconform-report.json)

exit $exitCode
