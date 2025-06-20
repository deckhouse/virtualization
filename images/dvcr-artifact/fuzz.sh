#!/bin/bash

# Copyright 2025 Flant JSC
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

set -e

fuzzTime=${FUZZ_TIME:-2m}
files=$(grep -r --include='**_test.go' --files-with-matches 'func Fuzz' .)
for file in ${files}
do
  funcs=$(grep -o 'func \(Fuzz\w*\)' $file)
  for func in ${funcs}
  do
    echo "Fuzzing $func in $file"
    parentDir=$(dirname $file)
    go test $parentDir -fuzz=$func -fuzztime=${fuzzTime} -cover -parallel=1 -v
  done
done

