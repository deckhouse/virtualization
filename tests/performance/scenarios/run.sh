#!/usr/bin/env bash

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

cleanup() {
  echo "Cleanup"
  echo "Exiting..."
  exit 0
}

trap cleanup SIGINT SIGTERM

# Create vd from vi+pvc
echo "Create vd from vi+pvc"

bootstrapper apply -c 10 -n perf -R disks -r performance
bootstrapper apply -c 5 -n perf -R vms -r performance

up=false

while $up; do
  ready=$(kubectl -n perf get vd | grep "Ready" | wc -l)
  all=$(kubectl -n perf get vd -o name | wc -l)
  if [ $ready -eq $all ]; then
    up=true
  fi
  echo "Waiting for vds to be ready..."
  echo "Ready: $ready/$all"
  echo "Waiting for 10 seconds..."
  sleep 10
done

task statistics:all

echo "migrations"

# tool for watch tcp connections and network connectivity
# https://fox.flant.com/team/virtualization/netchecker

SESSION="${USER}-perf"

tmux -2 kill-session -t "${SESSION}" || true
tmux -2 new-session -d -s "${SESSION}"

tmux new-window -t "$SESSION:1" -n "k9s"

# Setup 3-pane layout: [0] = vertical, [1] + [2] = horizontal
tmux select-window -t "$SESSION:1"
tmux split-window -h -t 0      # Pane 0 (left), Pane 1 (right)
tmux split-window -v -t 1      # Pane 1 (top), Pane 2 (bottom)

path="github/virtualization/tests/performance"
tmux select-pane -t 1
tmux send-keys "cd $path" C-m
tmux send-keys "task evicter run -t 10 -d 1h" C-m
tmux resize-pane -t 1 -x 50%

# some watch
tmux select-pane -t 2
tmux send-keys "cd $path" C-m
tmux resize-pane -t 2 -x 50%


echo "Restart virtualization-controller"
kubectl -n d8-virtualization rollout restart deployment virtualization-controller

echo "Restart kube-api-proxy"
kubectl -n d8-virtualization rollout restart deployment kube-api-proxy

