#!/bin/sh

echo "Mounts:"
mount | grep tmpfs | grep \(ro

echo "Environment variables:"
export

echo "Arguments:"
echo "$@"

if [ -n $IMPORTER_CERT_DIR ] ; then
  echo "IMPORTER_CERT_DIR is set. Remove well known certificates to properly test caBundle ..."
  rm -rf /etc/ca-certificates.conf /etc/ssl/certs/*
fi

echo
echo "Start importer ..."

/usr/local/bin/cdi-registry-importer "$@"
exitCode=$?
if [ "x$exitCode" != "x0" ] ; then
  # Add some messages for test purposes.
  echo "Complete with error"
  echo "Complete with error" > /dev/termination-log
  exit $exitCode
fi
