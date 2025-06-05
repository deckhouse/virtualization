#!/bin/bash

set -e
export GOMAXPROCS=1
export GOGC=10
export GOMEMLIMIT=1GiB
fuzzTime=${1:-10}
files=$(grep -r --include='**_test.go' --files-with-matches 'func Fuzz' .)
for file in ${files}
do
  funcs=$(grep -oP 'func \K(Fuzz\w*)' $file)
  for func in ${funcs}
  do
    echo "Fuzzing $func in $file"
    parentDir=$(dirname $file)
    go test $parentDir -run=$func -fuzz=$func -fuzztime=${fuzzTime}m -cover -v
  done
done

