#!/bin/sh

echo "Mounts:"
mount | grep tmpfs | grep \(ro

echo "Environment variables:"
export

echo "Arguments:"
echo "$@"

echo "Start importer"

if /usr/local/bin/cdi-registry-importer "$@" ; then
  echo "Complete, write termination message"
  #echo "Termination mesage on Complete" > /dev/termination-log
  #echo -e 'Complete\n{"size":11223123123, "image":"my-image"}' > /dev/termination-log
  echo '{ "source-image-size": 65011712, "source-image-virtual-size": 268435456, "source-image-format": "qcow2"}' > /dev/termination-log
fi
