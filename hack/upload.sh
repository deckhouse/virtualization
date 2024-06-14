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

function usage() {
   cat <<EOF
$0 namespace name filename

Program creates VirtualImage resource in specified namespace
and upload specified file to DVCR.

EOF
}

NS=$1
if [[ -z $NS ]] ; then
  usage && exit 1
fi
NAME=$2
if [[ -z $NAME ]] ; then
  usage && exit 1
fi
FILE=$3
if [[ -z $FILE ]] ; then
  usage && exit 1
fi


cat <<EOF | kubectl -n $NS apply -f -
apiVersion: virtualization.deckhouse.io/v1alpha2
kind: VirtualImage
metadata:
  name: $NAME
spec:
  storage: ContainerRegistry
  dataSource:
    type: Upload
EOF

tries=10

for (( i=1 ; i<=$tries ; i-- ))
do
  uploadCmd=$(kubectl -n $NS get virtualimage.virtualization.deckhouse.io $NAME -o json | jq '.status.uploadCommand // ""' -r)
  if [[ -n $uploadCmd ]] ; then break ; fi
  sleep 1
done

uploadCmd=$(echo "$uploadCmd ${FILE}" | sed "s/example.iso//")
echo $uploadCmd
eval "${uploadCmd}"