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
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strconv"

	yamlv3 "gopkg.in/yaml.v3"
	storagev1 "k8s.io/api/storage/v1"
	"k8s.io/cli-runtime/pkg/genericclioptions"
	"k8s.io/client-go/rest"
)

func GetConfig() (*Config, error) {
	cfg := "./default_config.yaml"
	if e, ok := os.LookupEnv("E2E_CONFIG"); ok {
		cfg = e
	}
	data, err := os.ReadFile(cfg)
	if err != nil {
		return nil, err
	}
	var conf Config

	if err := yamlv3.Unmarshal(data, &conf); err != nil {
		return nil, err
	}
	if err := conf.setEnvs(); err != nil {
		return nil, err
	}

	return &conf, nil
}

type Kustomize struct {
	APIVersion     string           `yaml:"apiVersion"`
	Labels         []KustomizeLabel `yaml:"labels"`
	Configurations []string         `yaml:"configurations"`
	Kind           string           `yaml:"kind"`
	Namespace      string           `yaml:"namespace"`
	NamePrefix     string           `yaml:"namePrefix"`
	Resources      []string         `yaml:"resources"`
}

type KustomizeLabel struct {
	IncludeSelectors bool              `yaml:"includeSelectors"`
	Pairs            map[string]string `yaml:"pairs"`
}

type Config struct {
	ClusterTransport ClusterTransport `yaml:"clusterTransport"`
	HelperImages     HelperImages     `yaml:"helperImages"`
	NamespaceSuffix  string           `yaml:"namespaceSuffix"`
	TestData         TestData         `yaml:"testData"`
	LogFilter        []string         `yaml:"logFilter"`
	CleanupResources []string         `yaml:"cleanupResources"`
	RegexpLogFilter  []regexp.Regexp  `yaml:"regexpLogFilter"`
	StorageClass     StorageClass
}

type TestData struct {
	AffinityToleration string `yaml:"affinityToleration"`
	ComplexTest        string `yaml:"complexTest"`
	DiskResizing       string `yaml:"diskResizing"`
	ImageHotplug       string `yaml:"imageHotplug"`
	VMLabelAnnotation  string `yaml:"vmLabelAnnotation"`
	VMMigration        string `yaml:"vmMigration"`
	VMMigrationCancel  string `yaml:"vmMigrationCancel"`
	VMEvacuation       string `yaml:"vmEvacuation"`
	VMDiskAttachment   string `yaml:"vmDiskAttachment"`
	VMRestoreForce     string `yaml:"vmRestoreForce"`
	VMRestoreSafe      string `yaml:"vmRestoreSafe"`
	VMVersions         string `yaml:"vmVersions"`
	VdSnapshots        string `yaml:"vdSnapshots"`
	Sshkey             string `yaml:"sshKey"`
	SSHUser            string `yaml:"sshUser"`
	VMVpc              string `yaml:"vmVpc"`
}

type StorageClass struct {
	DefaultStorageClass   *storagev1.StorageClass
	ImmediateStorageClass *storagev1.StorageClass
	TemplateStorageClass  *storagev1.StorageClass
}

type ClusterTransport struct {
	KubeConfig           string `yaml:"kubeConfig"`
	Token                string `yaml:"token"`
	Endpoint             string `yaml:"endpoint"`
	CertificateAuthority string `yaml:"certificateAuthority"`
	InsecureTLS          bool   `yaml:"insecureTls"`
}

func (c ClusterTransport) RestConfig() (*rest.Config, error) {
	configFlags := genericclioptions.ConfigFlags{}
	if c.KubeConfig != "" {
		configFlags.KubeConfig = &c.KubeConfig
	}
	if c.Token != "" {
		configFlags.BearerToken = &c.Token
	}
	if c.InsecureTLS {
		configFlags.Insecure = &c.InsecureTLS
	}
	if c.CertificateAuthority != "" {
		configFlags.CAFile = &c.CertificateAuthority
	}
	if c.Endpoint != "" {
		configFlags.APIServer = &c.Endpoint
	}
	return configFlags.ToRESTConfig()
}

type HelperImages struct {
	CurlImage string `yaml:"curlImage"`
}

func (c *Config) setEnvs() error {
	// ClusterTransport
	if e, ok := os.LookupEnv("E2E_CLUSTERTRANSPORT_KUBECONFIG"); ok {
		c.ClusterTransport.KubeConfig = e
	}
	if e, ok := os.LookupEnv("E2E_CLUSTERTRANSPORT_TOKEN"); ok {
		c.ClusterTransport.Token = e
	}
	if e, ok := os.LookupEnv("E2E_CLUSTERTRANSPORT_ENDPOINT"); ok {
		c.ClusterTransport.Endpoint = e
	}
	if e, ok := os.LookupEnv("E2E_CLUSTERTRANSPORT_CERTIFICATEAUTHORITY"); ok {
		c.ClusterTransport.CertificateAuthority = e
	}
	if e, ok := os.LookupEnv("E2E_CLUSTERTRANSPORT_INSECURETLS"); ok {
		v, err := strconv.ParseBool(e)
		if err != nil {
			return err
		}
		c.ClusterTransport.InsecureTLS = v
	}
	return nil
}

func (c *Config) GetTestCases() ([]string, error) {
	testDataValue := reflect.ValueOf(c.TestData)
	testDataType := reflect.TypeOf(c.TestData)
	excludedData := []string{"Sshkey", "SSHUser"}
	testCases := make([]string, 0, testDataType.NumField()-len(excludedData))

	if testDataType.Kind() == reflect.Struct {
		for i := 0; i < testDataType.NumField(); i++ {
			field := testDataType.Field(i)
			value := testDataValue.Field(i)
			if !slices.Contains(excludedData, field.Name) {
				testCases = append(testCases, fmt.Sprintf("%v", value.Interface()))
			}
		}
		return testCases, nil
	} else {
		return nil, errors.New("`config.TestData` it is not a structure")
	}
}

func (k *Kustomize) SetParams(filePath, namespace, namePrefix string) error {
	var kustomizeFile Kustomize

	data, readErr := os.ReadFile(filePath)
	if readErr != nil {
		return readErr
	}

	unmarshalErr := yamlv3.Unmarshal(data, &kustomizeFile)
	if unmarshalErr != nil {
		return unmarshalErr
	}

	fileDir := filepath.Dir(filePath)
	testCaseName := filepath.Base(fileDir)

	kustomizeFile.Namespace = namespace + "-" + testCaseName
	kustomizeFile.NamePrefix = namePrefix + "-"
	if len(kustomizeFile.Labels) > 0 {
		kustomizeFile.Labels[0].Pairs["id"] = namePrefix
	}
	updatedKustomizeFile, marshalErr := yamlv3.Marshal(&kustomizeFile)
	if marshalErr != nil {
		return marshalErr
	}

	writeErr := os.WriteFile(filePath, updatedKustomizeFile, 0o644)
	if writeErr != nil {
		return writeErr
	}

	return nil
}

func (k *Kustomize) GetNamespace(filePath string) (string, error) {
	var kustomizeFile Kustomize

	data, readErr := os.ReadFile(filePath)
	if readErr != nil {
		return "", fmt.Errorf("cannot get namespace from %s: %w", filePath, readErr)
	}

	unmarshalErr := yamlv3.Unmarshal(data, &kustomizeFile)
	if unmarshalErr != nil {
		return "", fmt.Errorf("cannot get namespace from %s: %w", filePath, unmarshalErr)
	}

	return kustomizeFile.Namespace, nil
}

func (k *Kustomize) ExcludeResource(filePath, resourceName string) error {
	var kustomizeFile Kustomize

	data, readErr := os.ReadFile(filePath)
	if readErr != nil {
		return readErr
	}

	unmarshalErr := yamlv3.Unmarshal(data, &kustomizeFile)
	if unmarshalErr != nil {
		return unmarshalErr
	}
	newResourceList := make([]string, 0, len(kustomizeFile.Resources))
	for _, v := range kustomizeFile.Resources {
		if v != resourceName {
			newResourceList = append(newResourceList, v)
		}
	}

	kustomizeFile.Resources = newResourceList
	updatedKustomizeFile, marshalErr := yamlv3.Marshal(&kustomizeFile)
	if marshalErr != nil {
		return marshalErr
	}

	writeErr := os.WriteFile(filePath, updatedKustomizeFile, 0o644)
	if writeErr != nil {
		return writeErr
	}

	return nil
}
