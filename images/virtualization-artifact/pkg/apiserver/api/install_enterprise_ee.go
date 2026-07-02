//go:build EE
// +build EE

/*
Copyright 2026 Flant JSC
Licensed under the Deckhouse Platform Enterprise Edition (EE) license. See https://github.com/deckhouse/deckhouse/blob/main/ee/LICENSE
*/

package api

import (
	"k8s.io/apiserver/pkg/registry/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"

	vmpoolstorage "github.com/deckhouse/virtualization-controller/pkg/apiserver/registry/vmpool/storage"
)

// installEnterpriseResources registers paid-edition subresources into the
// aggregated apiserver group. Compiled only in EE builds.
func installEnterpriseResources(resources map[string]rest.Storage, c client.Client) {
	poolStorage := vmpoolstorage.NewStorage(c)
	resources["virtualmachinepools"] = poolStorage
	resources["virtualmachinepools/scaledownwith"] = poolStorage.ScaleDownWithREST()
}
