package kubevirt

import (
	"context"
	"fmt"

	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const HotplugVolumesGate = "HotplugVolumes"

type KubeVirt struct {
	kubevirt virtv1.KubeVirt
}

func (kv *KubeVirt) GetKubeVirt() virtv1.KubeVirt {
	return kv.kubevirt
}

func (kv *KubeVirt) GetFeatureGates() []string {
	if conf := kv.kubevirt.Spec.Configuration; conf.DeveloperConfiguration != nil {
		return conf.DeveloperConfiguration.FeatureGates
	}
	return nil
}

func (kv *KubeVirt) HotplugVolumesEnabled() bool {
	return kv.IsEnabledFeatureGate(HotplugVolumesGate)
}

func (kv *KubeVirt) IsEnabledFeatureGate(featureGate string) bool {
	for _, fg := range kv.GetFeatureGates() {
		if fg == featureGate {
			return true
		}
	}
	return false
}

func New(ctx context.Context, cli client.Client, namespace string) (*KubeVirt, error) {
	kv, err := GetKubeVirt(ctx, cli, namespace)
	if err != nil {
		return nil, err
	}
	return &KubeVirt{kubevirt: *kv}, nil
}

func GetKubeVirt(ctx context.Context, cli client.Client, namespace string) (*virtv1.KubeVirt, error) {
	kubevirts := virtv1.KubeVirtList{}
	err := cli.List(ctx, &kubevirts, client.InNamespace(namespace))
	if err != nil {
		return nil, err
	}
	for _, kv := range kubevirts.Items {
		if kv.DeletionTimestamp == nil && kv.Status.Phase != "" {
			return &kv, nil
		}
	}
	return nil, fmt.Errorf("kubevirt not found")
}
