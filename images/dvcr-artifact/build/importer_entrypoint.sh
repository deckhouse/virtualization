#!/bin/sh

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

echo "Mounts:"
mount | grep tmpfs | grep \(ro

echo "Environment variables:"
export

echo "Arguments:"
echo "$@"

if [ "x$IMPORTER_CERT_DIR" != "x" ] ; then
  echo "IMPORTER_CERT_DIR is set. Remove well known certificates to properly test caBundle ..."
  rm -rf /etc/ca-certificates.conf /etc/ssl/certs/*
fi

echo
echo "Start importer ..."

/usr/local/bin/dvcr-importer "$@"
exitCode=$?
if [ "x$exitCode" != "x0" ] ; then
  # Add some messages for test purposes.
  echo "Complete with error"
  echo "Complete with error" > /dev/termination-log
  exit $exitCode
fi
