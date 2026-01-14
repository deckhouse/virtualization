package api

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/runtime/serializer"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
)

const Version = "v1alpha2"
const Group = "usb-gateway.virtualization.deckhouse.io"

var SchemeGroupVersion = schema.GroupVersion{Group: Group, Version: Version}

var (
	Scheme = runtime.NewScheme()
	Codecs = serializer.NewCodecFactory(Scheme)
)

var (
	SchemeBuilder = runtime.NewSchemeBuilder(addKnownTypes)
	AddToScheme   = SchemeBuilder.AddToScheme
)

func addKnownTypes(scheme *runtime.Scheme) error {
	scheme.AddKnownTypes(SchemeGroupVersion,
		&USBGatewayStatus{},
	)
	return nil
}

func init() {
	metav1.AddToGroupVersion(Scheme, SchemeGroupVersion)
	utilruntime.Must(AddToScheme(Scheme))
}
