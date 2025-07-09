package copy_custom_certificate

import (
	"context"
	"fmt"

	"github.com/deckhouse/module-sdk/pkg"
	"github.com/deckhouse/module-sdk/pkg/registry"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	"hooks/pkg/settings"
)

const (
	CustomCertificatesSnapshotName = "custom_certificates"
)

var _ = registry.RegisterFunc(config, reconcile)

var config = &pkg.HookConfig{
	Kubernetes: []pkg.KubernetesConfig{
		{
			Name:       CustomCertificatesSnapshotName,
			APIVersion: "v1",
			Kind:       "Secret",
			LabelSelector: &metav1.LabelSelector{
				MatchExpressions: []metav1.LabelSelectorRequirement{
					{
						Key:      "owner",
						Operator: metav1.LabelSelectorOpNotIn,
						Values:   []string{"helm"},
					},
				},
			},
			NamespaceSelector: &pkg.NamespaceSelector{
				NameSelector: &pkg.NameSelector{
					MatchNames: []string{"d8-system"},
				},
			},
			JqFilter:                     `{"name": .metadata.name, "data": .data}`,
			ExecuteHookOnSynchronization: ptr.To(false),
		},
	},
	OnAfterHelm: &pkg.OrderedConfig{Order: 10},
	Queue:       fmt.Sprintf("modules/%s", settings.ModuleName),
}

func reconcile(_ context.Context, input *pkg.HookInput) error {
	moduleName := settings.ModuleName

	snapshots := input.Snapshots.Get(CustomCertificatesSnapshotName)

	customCertificates := make(map[string]interface{})
	for _, snap := range snapshots {
		var result struct {
			Name string                 `json:"name"`
			Data map[string]interface{} `json:"data"`
		}

		if err := snap.UnmarshalTo(&result); err != nil {
			input.Logger.Error("Failed to unmarshal snapshot: %v", err)
			continue
		}
		customCertificates[result.Name] = result.Data
	}

	if len(customCertificates) == 0 {
		return nil
	}

	httpsMode := getHTTPSMode(moduleName, input)
	path := fmt.Sprintf("%s.internal.customCertificateData", moduleName)

	if httpsMode != "CustomCertificate" {
		input.Values.Remove(path)
		return nil
	}

	secretName := getFirstDefined(
		input,
		input.Values.Get(fmt.Sprintf("%s.https.customCertificate.secretName", moduleName)).String(),
		input.Values.Get("global.modules.https.customCertificate.secretName").String(),
	).(string)

	if secretName == "" {
		input.Values.Remove(path)
		return nil
	}

	secretData, ok := customCertificates[secretName]
	if !ok {
		input.Logger.Warn("Custom certificate secret name is configured, but secret d8-system/%s does not exist", secretName)
		input.Values.Remove(path)
		return nil
	}

	input.Values.Set(path, secretData)
	return nil
}

func getHTTPSMode(moduleName string, input *pkg.HookInput) string {
	modulePath := fmt.Sprintf("%s.https.mode", moduleName)
	globalPath := "global.modules.https.mode"

	mode := getFirstDefined(input, modulePath, globalPath)
	if mode == nil {
		return ""
	}

	if s, ok := mode.(string); ok {
		return s
	}

	return ""
}

func getFirstDefined(input *pkg.HookInput, paths ...string) interface{} {
	for _, path := range paths {
		if val := input.Values.Get(path); val.Exists() {
			return val
		}
	}

	return nil
}
