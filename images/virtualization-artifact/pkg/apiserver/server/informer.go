package server

import (
	"fmt"

	"k8s.io/client-go/rest"

	virtClient "github.com/deckhouse/virtualization/api/client/generated/clientset/versioned"
	virtInformers "github.com/deckhouse/virtualization/api/client/generated/informers/externalversions"
)

const (
	// we should never need to resync, since we're not worried about missing events,
	// and resync is actually for regular interval-based reconciliation these days,
	// so set the default resync interval to 0
	defaultResync = 0
)

func virtualizationInformerFactory(rest *rest.Config) (virtInformers.SharedInformerFactory, error) {
	client, err := virtClient.NewForConfig(rest)
	if err != nil {
		return nil, fmt.Errorf("unable to construct lister client: %w", err)
	}
	return virtInformers.NewSharedInformerFactory(client, defaultResync), nil
}
