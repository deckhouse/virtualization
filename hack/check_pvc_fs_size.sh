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

SC_NAME=${SC_NAME:-linstor-thin-data-r1}

function usage() {
cat <<EOF
$0 namespace size

Create PVC with the specified size and mount it to the Pod.
Print fs info to see available size after PVC mounting.

Note: size is a Mebibytes count. Specify 30 to create PVC with request storage 30Mi.

Set SC_NAME to specify another storage class name.
EOF
}

NS=$1
if [[ -z $NS ]] ; then
  usage && exit 1
fi

SIZE=$2
if [[ -z $SIZE ]] ; then
  usage && exit 1
fi

echo "Use storageClass $SC_NAME, namespace $NS"
echo "PVC size ${SIZE}Mi"

(
kubectl -n $NS delete po fs-size || true
kubectl -n $NS delete pvc fs-size || true
) >/dev/null 2>&1

# Create PVC with specified size and a Pod to check FS size.
cat <<EOF | kubectl -n $NS apply -f -
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: fs-size
spec:
  accessModes:
  - ReadWriteOnce
  resources:
    requests:
      storage: ${SIZE}Mi
  storageClassName: $SC_NAME
  volumeMode: Filesystem
---
apiVersion: v1
kind: Pod
metadata:
  name: fs-size
spec:
  containers:
  - image: alpine:3.17
    imagePullPolicy: IfNotPresent
    name: main
    command: ["ash", "-c", "df -m /scratch > /dev/termination-log"]
    volumeMounts:
    - mountPath: /scratch
      name: cdi-scratch-vol
  dnsPolicy: ClusterFirst
  restartPolicy: OnFailure
  serviceAccount: default
  serviceAccountName: default
  volumes:
  - name: cdi-scratch-vol
    persistentVolumeClaim:
      claimName: fs-size
EOF

tries=15

for (( i=1 ; i<=$tries ; i-- ))
do
  msg=$(kubectl -n $NS get po fs-size -o json | jq '.status.containerStatuses[0].state.terminated.message // ""' -r)
  if [[ -n $msg ]] ; then break ; fi
  sleep 1
done

(
kubectl -n $NS delete po fs-size
kubectl -n $NS delete pvc fs-size
) >/dev/null 2>&1

echo "$msg"