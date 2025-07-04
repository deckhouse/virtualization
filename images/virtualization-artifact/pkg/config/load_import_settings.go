/*
Copyright 2024 Flant JSC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

     http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package config

import (
	"encoding/json"
	"fmt"
	"os"

	corev1 "k8s.io/api/core/v1"

	"github.com/deckhouse/virtualization-controller/pkg/common"
)

const (
	ProvisioningPodLimitsVar   = "PROVISIONING_POD_LIMITS"
	ProvisioningPodRequestsVar = "PROVISIONING_POD_REQUESTS"
)

type ImportSettings struct {
	ImporterImage string
	UploaderImage string
	BounderImage  string
	ExporterImage string
	Requirements  corev1.ResourceRequirements
}

func LoadImportSettingsFromEnv() (ImportSettings, error) {
	var settings ImportSettings
	var err error
	settings.ImporterImage, err = GetRequiredEnvVar(common.ImporterPodImageNameVar)
	if err != nil {
		return ImportSettings{}, err
	}

	settings.UploaderImage, err = GetRequiredEnvVar(common.UploaderPodImageNameVar)
	if err != nil {
		return ImportSettings{}, err
	}

	settings.BounderImage, err = GetRequiredEnvVar(common.BounderPodImageNameVar)
	if err != nil {
		return ImportSettings{}, err
	}

	settings.ExporterImage, err = GetRequiredEnvVar(common.ExporterPodImageNameVar)
	if err != nil {
		return ImportSettings{}, err
	}

	limits := os.Getenv(ProvisioningPodLimitsVar)
	if limits != "" {
		err = json.Unmarshal([]byte(limits), &settings.Requirements.Limits)
		if err != nil {
			return ImportSettings{}, err
		}
	}
	requests := os.Getenv(ProvisioningPodRequestsVar)
	if requests != "" {
		err = json.Unmarshal([]byte(requests), &settings.Requirements.Requests)
		if err != nil {
			return ImportSettings{}, err
		}
	}

	return settings, nil
}

func GetRequiredEnvVar(name string) (string, error) {
	val := os.Getenv(name)
	if val == "" {
		return "", fmt.Errorf("environment variable %q undefined", name)
	}
	return val, nil
}
