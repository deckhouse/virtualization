package install

import (
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

// Install registers the API group and adds types to a scheme
func Install(scheme *runtime.Scheme) {
	utilruntime.Must(v1alpha2.AddToScheme(scheme))
	utilruntime.Must(scheme.SetVersionPriority(v1alpha2.SchemeGroupVersion))
}
