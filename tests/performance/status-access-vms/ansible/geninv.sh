#!/bin/bash

NAMESPACE=test-perf

vms=$(kubectl -n $NAMESPACE get vm -o=jsonpath='{range .items[*]}{.metadata.name}{"\t"}{.status.ipAddress}{"\n"}{end}')
mkdir -p inventory
inventory_file="inventory/hosts.yml"
echo "---
all:
  hosts:" > $inventory_file

while IFS=$'\t' read -r vm_name vm_ip; do
    echo "    ${vm_name}:" >> $inventory_file
    echo "      ansible_ssh_host: ${vm_ip}" >> $inventory_file
done <<< "$vms"
