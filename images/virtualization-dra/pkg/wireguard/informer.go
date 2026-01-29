package wireguard

import (
	"time"

	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/client-go/tools/cache"

	drav1alpha1 "github.com/deckhouse/virtualization-dra/api/client/generated/clientset/versioned/typed/api/v1alpha1"
	vdraapi "github.com/deckhouse/virtualization-dra/api/v1alpha1"
)

func NewSharedIndexInformer(namespace string, client drav1alpha1.UsbgatewayV1alpha1Interface, resyncPeriod time.Duration) cache.SharedIndexInformer {
	lw := cache.NewListWatchFromClient(client.RESTClient(), "wireguardsystemnetworks", namespace, fields.Everything())
	return cache.NewSharedIndexInformer(lw, &vdraapi.WireguardSystemNetwork{}, resyncPeriod, cache.Indexers{})
}
