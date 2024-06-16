#!/usr/bin/env bash

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

NAMESPACE=kproxy
SERVICE_NAME=test-admission-webhook
CN="api proxying tests for validating webhook"
OUTDIR=proxy-certs

COMMON_NAME=${SERVICE_NAME}.${NAMESPACE}

set -eo pipefail

echo =================================================================
echo THIS SCRIPT IS NOT SECURE! USE IT ONLY FOR DEMONSTATION PURPOSES.
echo =================================================================
echo

mkdir -p ${OUTDIR} && cd ${OUTDIR}

if [[ -e ca.csr  ]] ; then
  read -p "Regenerate certificates? (yes/no) [no]: "
  if [[ ! $REPLY =~ ^[Yy][Ee][Ss]$ ]]
  then
    exit 0
  fi
fi

RM_FILES="ca* cert*"
echo ">>> Remove ${RM_FILES}"
rm -f $RM_FILES

echo ">>> Generate CA key and certificate"
cat <<EOF | cfssl gencert -initca - | cfssljson -bare ca
{
  "CN": "${CN}",
  "key": {
    "algo": "rsa",
    "size": 2048
  }
}
EOF


CFSSL_CONFIG=$(cat <<EOF
{
  "signing": {
    "default": {
      "expiry": "8760h"
    },
    "profiles": {
      "server": {
        "usages": [
          "signing",
          "digital signing",
          "key encipherment",
          "server auth"
        ],
        "expiry": "8760h"
      }
    }
  }
}
EOF
)

echo ">>> Generate cert.key and cert.crt"
cat <<EOF | cfssl gencert -ca ca.pem -ca-key ca-key.pem -config <(echo "$CFSSL_CONFIG") -profile=server - | cfssljson -bare tls
{
  "CN": "${COMMON_NAME}.svc",
  "key": {
    "algo": "rsa",
    "size": 2048
  },
  "hosts": [
    "${COMMON_NAME}",
    "${COMMON_NAME}.svc",
    "${COMMON_NAME}.svc.cluster.local"
  ]
}
EOF
