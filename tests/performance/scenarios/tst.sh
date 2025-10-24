#!/usr/bin/env bash

echo "Tst return"

exit_trap() {
  echo ""
  echo "Cleanup"
  echo "Exiting..."
  exit 0
}

trap exit_trap SIGINT SIGTERM

while true; do  
  for i in {1..5}; do
    echo "Iteration $i"
    if [ $i -eq 3 ]; then
      echo "$i = 3"
      echo "Break"
      return 0
    fi
  done
done