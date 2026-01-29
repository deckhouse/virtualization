package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const WireguardSystemNetworkKind = "WireguardSystemNetwork"

// +genclient
// +kubebuilder:object:root=true
// +kubebuilder:metadata:labels={heritage=deckhouse,module=virtualization}
// +kubebuilder:resource:scope=Namespaced,shortName={wsn},singular=wireguardsystemnetwork
// +kubebuilder:subresource:status
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type WireguardSystemNetwork struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   WireguardSystemNetworkSpec   `json:"spec"`
	Status WireguardSystemNetworkStatus `json:"status,omitempty"`
}

type WireguardSystemNetworkSpec struct {
	// +kubebuilder:validation:Format=cidr
	CIDR                string `json:"cidr"`
	ListenPort          int    `json:"listenPort"`
	InterfaceName       string `json:"interfaceName"`
	PersistentKeepalive int    `json:"persistentKeepalive,omitempty"`
}

type WireguardSystemNetworkStatus struct {
	AllocatedIPs []AllocatedIP  `json:"allocatedIPs"`
	NodeSettings []NodeSettings `json:"nodeSettings"`
}

type AllocatedIP struct {
	Node string `json:"node"`
	IP   string `json:"ip"`
}

type NodeSettings struct {
	Node       string `json:"node"`
	PrivateKey string `json:"privateKey"`
	PublicKey  string `json:"publicKey"`
	Endpoint   string `json:"endpoint"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
type WireguardSystemNetworkList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata"`

	Items []WireguardSystemNetwork `json:"items"`
}
