#!/bin/sh

echo "Hello!"

function sigterm() {
  echo Terminating by SIGTERM ...
  exit 1
}
trap sigterm SIGTERM

file=${FILE:-/tmp/exit}

for i in $(seq 1 600) ; do
  sleep 1
  if [[ -f $file ]] ; then
    code=$(cat $file)
    echo "Detect ${file}, exit with code ${code}"
    exit $code
  fi
  echo "Please, 'echo 0 > ${file}' to stop the Pod."
done

echo "Exit with error"
exit 1
