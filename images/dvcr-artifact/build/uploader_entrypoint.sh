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

echo
echo "Start uploader ..."

/usr/local/bin/dvcr-uploader "$@"

exitCode=$?
if [ "x$exitCode" != "x0" ] ; then
  # Add some messages for test purposes.
  echo "Complete with error"
  echo "Complete with error" > /dev/termination-log
  exit $exitCode
fi
