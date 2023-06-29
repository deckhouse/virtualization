#!/bin/sh

echo "Mounts:"
mount | grep tmpfs | grep \(ro

echo "Environment variables:"
export

exec /usr/local/bin/cdi-registry-importer
