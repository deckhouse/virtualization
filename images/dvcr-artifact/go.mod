module github.com/deckhouse/virtualization-controller/dvcr-importers

go 1.22.6

require (
	github.com/containers/image/v5 v5.25.0
	github.com/distribution/reference v0.5.0
	github.com/docker/cli v25.0.3+incompatible
	github.com/golang/snappy v0.0.4
	github.com/google/go-containerregistry v0.15.2
	github.com/hashicorp/go-multierror v1.1.1
	github.com/openshift/library-go v0.0.0-20230328115725-6ed98e0ed0b9
	github.com/pkg/errors v0.9.1
	github.com/prometheus/client_golang v1.17.0
	github.com/prometheus/client_model v0.5.0
	golang.org/x/net v0.23.0
	golang.org/x/sync v0.6.0
	k8s.io/klog/v2 v2.90.1
	kubevirt.io/containerized-data-importer v0.0.0-00010101000000-000000000000
	kubevirt.io/containerized-data-importer-api v1.57.0-alpha1
)

require (
	cloud.google.com/go v0.110.7 // indirect
	cloud.google.com/go/compute v1.23.0 // indirect
	cloud.google.com/go/compute/metadata v0.2.3 // indirect
	cloud.google.com/go/iam v1.1.1 // indirect
	cloud.google.com/go/storage v1.30.1 // indirect
	github.com/BurntSushi/toml v1.3.2 // indirect
	github.com/Microsoft/go-winio v0.6.1 // indirect
	github.com/aws/aws-sdk-go v1.34.0 // indirect
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/cespare/xxhash/v2 v2.2.0 // indirect
	github.com/checkpoint-restore/go-criu/v5 v5.3.0 // indirect
	github.com/cilium/ebpf v0.7.0 // indirect
	github.com/containerd/console v1.0.3 // indirect
	github.com/containerd/stargz-snapshotter/estargz v0.15.1 // indirect
	github.com/containers/libtrust v0.0.0-20230121012942-c1716e8a8d01 // indirect
	github.com/containers/ocicrypt v1.1.9 // indirect
	github.com/containers/storage v1.50.0 // indirect
	github.com/coreos/go-systemd/v22 v22.3.2 // indirect
	github.com/cpuguy83/go-md2man/v2 v2.0.2 // indirect
	github.com/cyphar/filepath-securejoin v0.2.4 // indirect
	github.com/davecgh/go-spew v1.1.1 // indirect
	github.com/docker/distribution v2.8.2-beta.1+incompatible // indirect
	github.com/docker/docker v25.0.6+incompatible // indirect
	github.com/docker/docker-credential-helpers v0.8.1 // indirect
	github.com/docker/go-connections v0.5.0 // indirect
	github.com/docker/go-metrics v0.0.1 // indirect
	github.com/docker/go-units v0.5.0 // indirect
	github.com/emicklei/go-restful/v3 v3.9.0 // indirect
	github.com/evanphx/json-patch v4.12.0+incompatible // indirect
	github.com/evanphx/json-patch/v5 v5.6.0 // indirect
	github.com/ghodss/yaml v1.0.0 // indirect
	github.com/go-logr/logr v1.2.4 // indirect
	github.com/go-openapi/jsonpointer v0.19.6 // indirect
	github.com/go-openapi/jsonreference v0.20.2 // indirect
	github.com/go-openapi/swag v0.22.10 // indirect
	github.com/godbus/dbus/v5 v5.0.6 // indirect
	github.com/gogo/protobuf v1.3.2 // indirect
	github.com/golang/groupcache v0.0.0-20210331224755-41bb18bfe9da // indirect
	github.com/golang/protobuf v1.5.3 // indirect
	github.com/google/gnostic v0.5.7-v3refs // indirect
	github.com/google/go-cmp v0.6.0 // indirect
	github.com/google/gofuzz v1.2.0 // indirect
	github.com/google/uuid v1.6.0 // indirect
	github.com/googleapis/enterprise-certificate-proxy v0.2.3 // indirect
	github.com/googleapis/gax-go/v2 v2.11.0 // indirect
	github.com/gorilla/mux v1.8.1 // indirect
	github.com/hashicorp/errwrap v1.1.0 // indirect
	github.com/jmespath/go-jmespath v0.0.0-20160803190731-bd40a432e4c7 // indirect
	github.com/josharian/intern v1.0.0 // indirect
	github.com/json-iterator/go v1.1.12 // indirect
	github.com/klauspost/compress v1.17.7 // indirect
	github.com/klauspost/pgzip v1.2.6 // indirect
	github.com/kubernetes-csi/external-snapshotter/client/v6 v6.0.1 // indirect
	github.com/mailru/easyjson v0.7.7 // indirect
	github.com/matttproud/golang_protobuf_extensions v1.0.4 // indirect
	github.com/mitchellh/go-homedir v1.1.0 // indirect
	github.com/moby/sys/mountinfo v0.7.1 // indirect
	github.com/modern-go/concurrent v0.0.0-20180306012644-bacd9c7ef1dd // indirect
	github.com/modern-go/reflect2 v1.0.2 // indirect
	github.com/mrunalp/fileutils v0.5.1 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/opencontainers/go-digest v1.0.0 // indirect
	github.com/opencontainers/image-spec v1.1.0 // indirect
	github.com/opencontainers/runc v1.1.12 // indirect
	github.com/opencontainers/runtime-spec v1.2.0 // indirect
	github.com/opencontainers/selinux v1.11.0 // indirect
	github.com/openshift/api v0.0.0-20230406152840-ce21e3fe5da2 // indirect
	github.com/openshift/custom-resource-status v1.1.2 // indirect
	github.com/ovirt/go-ovirt v0.0.0-20210809163552-d4276e35d3db // indirect
	github.com/ovirt/go-ovirt-client v0.9.0 // indirect
	github.com/ovirt/go-ovirt-client-log-klog v1.0.0 // indirect
	github.com/ovirt/go-ovirt-client-log/v2 v2.2.0 // indirect
	github.com/prometheus/common v0.44.0 // indirect
	github.com/prometheus/procfs v0.11.1 // indirect
	github.com/russross/blackfriday/v2 v2.1.0 // indirect
	github.com/seccomp/libseccomp-golang v0.9.2-0.20220502022130-f33da4d89646 // indirect
	github.com/sirupsen/logrus v1.9.3 // indirect
	github.com/syndtr/gocapability v0.0.0-20200815063812-42c35b437635 // indirect
	github.com/ulikunitz/xz v0.5.11 // indirect
	github.com/urfave/cli v1.22.12 // indirect
	github.com/vbatts/tar-split v0.11.5 // indirect
	github.com/vishvananda/netlink v1.1.0 // indirect
	github.com/vishvananda/netns v0.0.0-20191106174202-0a2b9b5464df // indirect
	github.com/vmware/govmomi v0.23.1 // indirect
	go.etcd.io/bbolt v1.3.9 // indirect
	go.opencensus.io v0.24.0 // indirect
	golang.org/x/crypto v0.21.0 // indirect
	golang.org/x/mod v0.15.0 // indirect
	golang.org/x/oauth2 v0.13.0 // indirect
	golang.org/x/sys v0.19.0 // indirect
	golang.org/x/term v0.19.0 // indirect
	golang.org/x/text v0.14.0 // indirect
	golang.org/x/time v0.3.0 // indirect
	golang.org/x/tools v0.14.0 // indirect
	golang.org/x/xerrors v0.0.0-20220907171357-04be3eba64a2 // indirect
	google.golang.org/api v0.126.0 // indirect
	google.golang.org/appengine v1.6.8 // indirect
	google.golang.org/genproto v0.0.0-20230530153820-e85fd2cbaebc // indirect
	google.golang.org/genproto/googleapis/rpc v0.0.0-20230530153820-e85fd2cbaebc // indirect
	google.golang.org/grpc v1.56.3 // indirect
	google.golang.org/protobuf v1.33.0 // indirect
	gopkg.in/inf.v0 v0.9.1 // indirect
	gopkg.in/square/go-jose.v2 v2.6.0 // indirect
	gopkg.in/yaml.v2 v2.4.0 // indirect
	gopkg.in/yaml.v3 v3.0.1 // indirect
	k8s.io/api v0.26.3 // indirect
	k8s.io/apiextensions-apiserver v0.26.1 // indirect
	k8s.io/apimachinery v0.26.3 // indirect
	k8s.io/apiserver v0.26.1 // indirect
	k8s.io/client-go v12.0.0+incompatible // indirect
	k8s.io/kube-openapi v0.0.0-20221012153701-172d655c2280 // indirect
	k8s.io/utils v0.0.0-20230313181309-38a27ef9d749 // indirect
	kubevirt.io/controller-lifecycle-operator-sdk/api v0.0.0-20220329064328-f3cc58c6ed90 // indirect
	libguestfs.org/libnbd v1.11.5 // indirect
	sigs.k8s.io/controller-runtime v0.14.6 // indirect
	sigs.k8s.io/json v0.0.0-20221116044647-bc3834ca7abd // indirect
	sigs.k8s.io/structured-merge-diff/v4 v4.2.3 // indirect
	sigs.k8s.io/yaml v1.3.0 // indirect
)

replace (
	github.com/aws/aws-sdk-go => github.com/aws/aws-sdk-go v1.15.77
	github.com/chzyer/logex => github.com/chzyer/logex v1.2.1
	github.com/openshift/api => github.com/openshift/api v0.0.0-20230406152840-ce21e3fe5da2
	github.com/openshift/client-go => github.com/openshift/client-go v0.0.0-20230324103026-3f1513df25e0
	github.com/openshift/library-go => github.com/mhenriks/library-go v0.0.0-20230310153733-63d38b55bd5a
	github.com/operator-framework/operator-lifecycle-manager => github.com/operator-framework/operator-lifecycle-manager v0.0.0-20190128024246-5eb7ae5bdb7a

	k8s.io/api => k8s.io/api v0.26.1
	k8s.io/apiextensions-apiserver => k8s.io/apiextensions-apiserver v0.26.1
	k8s.io/apimachinery => k8s.io/apimachinery v0.26.1
	k8s.io/apiserver => k8s.io/apiserver v0.26.1
	k8s.io/cli-runtime => k8s.io/cli-runtime v0.26.1
	k8s.io/cloud-provider => k8s.io/cloud-provider v0.26.1
	k8s.io/cluster-bootstrap => k8s.io/cluster-bootstrap v0.26.1
	k8s.io/code-generator => k8s.io/code-generator v0.26.1
	k8s.io/component-base => k8s.io/component-base v0.26.1
	k8s.io/component-helpers => k8s.io/component-helpers v0.26.1
	k8s.io/controller-manager => k8s.io/controller-manager v0.26.1
	k8s.io/cri-api => k8s.io/cri-api v0.26.1
	k8s.io/csi-translation-lib => k8s.io/csi-translation-lib v0.26.1
	k8s.io/dynamic-resource-allocation => dynamic-resource-allocation v0.26.1
	k8s.io/kube-aggregator => k8s.io/kube-aggregator v0.26.1
	k8s.io/kube-controller-manager => k8s.io/kube-controller-manager v0.26.1
	k8s.io/kube-proxy => k8s.io/kube-proxy v0.26.1
	k8s.io/kube-scheduler => k8s.io/kube-scheduler v0.26.1
	k8s.io/kubectl => k8s.io/kubectl v0.26.1
	k8s.io/kubelet => k8s.io/kubelet v0.26.1
	k8s.io/legacy-cloud-providers => k8s.io/legacy-cloud-providers v0.26.1
	k8s.io/metrics => k8s.io/metrics v0.26.1
	k8s.io/mount-utils => k8s.io/mount-utils v0.26.1
	k8s.io/pod-security-admission => k8s.io/pod-security-admission v0.26.1
	k8s.io/sample-apiserver => k8s.io/sample-apiserver v0.26.1
	k8s.io/sample-cli-plugin => k8s.io/sample-cli-plugin v0.26.1
	k8s.io/sample-controller => k8s.io/sample-controller v0.26.1

	kubevirt.io/controller-lifecycle-operator-sdk/api => kubevirt.io/controller-lifecycle-operator-sdk/api v0.0.0-20220329064328-f3cc58c6ed90
)

replace (
	k8s.io/client-go => k8s.io/client-go v0.26.1
	kubevirt.io/containerized-data-importer => github.com/deckhouse/3p-containerized-data-importer v1.54.1-0.20231108105404-81f1cea8c40d // branch: virtualization-controller
)
