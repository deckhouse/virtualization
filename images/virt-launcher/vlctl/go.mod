module vlctl

go 1.25.12

require (
	github.com/spf13/cobra v1.9.1
	github.com/spf13/pflag v1.0.6
	google.golang.org/grpc v1.79.3
	google.golang.org/protobuf v1.36.10
	gopkg.in/yaml.v3 v3.0.1
	k8s.io/apimachinery v0.34.2
	kubevirt.io/api v0.0.0-20250930144221-aaa67e9803df
)

require (
	github.com/fxamacker/cbor/v2 v2.9.0 // indirect
	github.com/go-logr/logr v1.4.3 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/inconshreveable/mousetrap v1.1.0 // indirect
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.3-0.20250322232337-35a7c28c31ee // indirect
	github.com/openshift/custom-resource-status v1.1.2 // indirect
	github.com/x448/float16 v0.8.4 // indirect
	go.yaml.in/yaml/v2 v2.4.2 // indirect
	golang.org/x/net v0.53.0 // indirect
	golang.org/x/sys v0.45.0 // indirect
	golang.org/x/text v0.37.0 // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20251202230838-ff82c1b0f217 // indirect
	gopkg.in/inf.v0 v0.9.1 // indirect
	k8s.io/api v0.34.2 // indirect
	k8s.io/apiextensions-apiserver v0.32.5 // indirect
	k8s.io/klog/v2 v2.130.1 // indirect
	k8s.io/utils v0.0.0-20250604170112-4c0f3b243397 // indirect
	kubevirt.io/containerized-data-importer-api v1.60.3-0.20241105012228-50fbed985de9 // indirect
	kubevirt.io/controller-lifecycle-operator-sdk/api v0.0.0-20220329064328-f3cc58c6ed90 // indirect
	sigs.k8s.io/json v0.0.0-20241014173422-cfa47c3a1cc8 // indirect
	sigs.k8s.io/randfill v1.0.0 // indirect
	sigs.k8s.io/structured-merge-diff/v6 v6.3.0 // indirect
)

replace (
	github.com/nxadm/tail => github.com/nxadm/tail v0.0.0-20211216163028-4472660a31a6
	github.com/openshift/api => github.com/openshift/api v0.0.0-20240625084701-0689f006bcde
	github.com/openshift/client-go => github.com/openshift/client-go v0.0.0-20240528061634-b054aa794d87
	github.com/operator-framework/operator-lifecycle-manager => github.com/operator-framework/operator-lifecycle-manager v0.0.0-20190128024246-5eb7ae5bdb7a

	k8s.io/api => k8s.io/api v0.34.2
	k8s.io/apiextensions-apiserver => k8s.io/apiextensions-apiserver v0.34.2
	k8s.io/apimachinery => k8s.io/apimachinery v0.34.2
	k8s.io/client-go => k8s.io/client-go v0.34.2
	k8s.io/component-base => k8s.io/component-base v0.34.2
	k8s.io/klog => k8s.io/klog v0.4.0
	k8s.io/kube-openapi => k8s.io/kube-openapi v0.0.0-20250710124328-f3f2b991d03b

	kubevirt.io/client-go => kubevirt.io/client-go v1.6.1
)

// CVE Replaces
replace (
	github.com/golang/glog => github.com/golang/glog v1.2.4 // CVE-2024-45339
	golang.org/x/net => golang.org/x/net v0.55.0
)
