//go:build EE
// +build EE

/*
Copyright 2025 Flant JSC
Licensed under the Deckhouse Platform Enterprise Edition (EE) license. See https://github.com/deckhouse/deckhouse/blob/main/ee/LICENSE
*/

package main

import (
	_ "hooks/pkg/hooks/tls-certificates-audit"
)
