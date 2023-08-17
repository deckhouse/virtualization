#!/bin/sh

echo "Mounts:"
mount | grep tmpfs | grep \(ro

echo "Environment variables:"
export

echo "Arguments:"
echo "$@"

echo
echo "Start uploader ..."

/usr/local/bin/cdi-registry-uploader "$@"
exitCode=$?
if [ "x$exitCode" != "x0" ] ; then
  # Add some messages for test purposes.
  echo "Complete with error"
  echo "Complete with error" > /dev/termination-log
  exit $exitCode
fi
