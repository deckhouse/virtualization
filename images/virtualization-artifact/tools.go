//go:build tools
// +build tools

package tools

// This file only needed to hold tool dependencies in go.mod.

import (
	_ "k8s.io/code-generator"
	_ "k8s.io/kube-openapi/cmd/openapi-gen"
)
