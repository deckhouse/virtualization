#!/usr/bin/env bash

kubectl get vm -A | grep Running | awk '{print $1, $2}' | while read -r namespace vm; do
  kubectl -n "$namespace" patch vm "$vm" --type=merge -p '{"spec":{"runPolicy":"AlwaysOff"}}'
done

