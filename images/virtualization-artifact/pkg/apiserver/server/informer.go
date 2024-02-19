package server

import (
	"fmt"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

const (
	// we should never need to resync, since we're not worried about missing events,
	// and resync is actually for regular interval-based reconciliation these days,
	// so set the default resync interval to 0
	defaultResync = 0
)

func informerFactory(rest *rest.Config) (informers.SharedInformerFactory, error) {
	client, err := kubernetes.NewForConfig(rest)
	if err != nil {
		return nil, fmt.Errorf("unable to construct lister client: %v", err)
	}
	return informers.NewSharedInformerFactory(client, defaultResync), nil
}
