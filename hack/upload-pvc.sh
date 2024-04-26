#!/bin/bash

function usage() {
   cat <<EOF
$0 namespace name filename

Program creates VirtualImage resource in specified namespace
and upload specified file using Kubernetes storage (PVC).

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
  storage: Kubernetes
  persistentVolumeClaim:
    storageClass: linstor-thick-data-r3
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