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
	"log"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"

	yamlv3 "gopkg.in/yaml.v3"
	storagev1 "k8s.io/api/storage/v1"

	gt "github.com/deckhouse/virtualization/tests/e2e/git"
	kc "github.com/deckhouse/virtualization/tests/e2e/kubectl"
)

var (
	conf    *Config
	git     gt.Git
	kubectl kc.Kubectl
)

func init() {
	var err error
	if conf, err = GetConfig(); err != nil {
		log.Fatal(err)
	}
	if git, err = gt.NewGit(); err != nil {
		log.Fatal(err)
	}
	if kubectl, err = kc.NewKubectl(kc.KubectlConf(conf.ClusterTransport)); err != nil {
		log.Fatal(err)
	}
}

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

type ModuleConfig struct {
	ApiVersion string   `yaml:"apiVersion"`
	Kind       string   `yaml:"kind"`
	Metadata   Metadata `yaml:"metadata"`
	Spec       Spec     `yaml:"spec"`
}

type Metadata struct {
	Name string `yaml:"name"`
}

type Spec struct {
	Enabled  bool     `yaml:"enabled"`
	Settings Settings `yaml:"settings"`
	Version  int      `yaml:"version"`
}

type Settings struct {
	Loglevel            string   `yaml:"logLevel,omitempty"`
	VirtualMachineCIDRs []string `yaml:"virtualMachineCIDRs"`
	Dvcr                Dvcr     `yaml:"dvcr"`
	HighAvailability    bool     `yaml:"highAvailability,omitempty"`
}

type Dvcr struct {
	Storage Storage `yaml:"storage"`
}

type Storage struct {
	PersistentVolumeClaim map[string]string `yaml:"persistentVolumeClaim"`
	Type                  string            `yaml:"type"`
}

type Kustomize struct {
	ApiVersion     string           `yaml:"apiVersion"`
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
	Disks            DisksConf        `yaml:"disks"`
	VM               VmConf           `yaml:"vm"`
	Ipam             IpamConf         `yaml:"ipam"`
	HelperImages     HelperImages     `yaml:"helperImages"`
	Namespace        string           `yaml:"namespaceSuffix"`
	TestData         TestData         `yaml:"testData"`
	LogFilter        []string         `yaml:"logFilter"`
	RegexpLogFilter  []string         `yaml:"regexpLogFilter"`
	StorageClass     StorageClass
}

type TestData struct {
	AffinityToleration    string `yaml:"affinityToleration"`
	ComplexTest           string `yaml:"complexTest"`
	Connectivity          string `yaml:"connectivity"`
	DiskResizing          string `yaml:"diskResizing"`
	SizingPolicy          string `yaml:"sizingPolicy"`
	ImporterNetworkPolicy string `yaml:"importerNetworkPolicy"`
	ImageHotplug          string `yaml:"imageHotplug"`
	ImagesCreation        string `yaml:"imagesCreation"`
	VmConfiguration       string `yaml:"vmConfiguration"`
	VmLabelAnnotation     string `yaml:"vmLabelAnnotation"`
	VmMigration           string `yaml:"vmMigration"`
	VmDiskAttachment      string `yaml:"vmDiskAttachment"`
	VdSnapshots           string `yaml:"vdSnapshots"`
	Sshkey                string `yaml:"sshKey"`
	SshUser               string `yaml:"sshUser"`
}

type StorageClass struct {
	VolumeBindingMode storagev1.VolumeBindingMode
}

type ClusterTransport struct {
	KubeConfig           string `yaml:"kubeConfig"`
	Token                string `yaml:"token"`
	Endpoint             string `yaml:"endpoint"`
	CertificateAuthority string `yaml:"certificateAuthority"`
	InsecureTls          bool   `yaml:"insecureTls"`
}

type DisksConf struct {
	UploadHelperImage string `yaml:"uploadHelperImage"`
	CviTestDataDir    string `yaml:"cviTestDataDir"`
	ViTestDataDir     string `yaml:"viTestDataDir"`
	VdTestDataDir     string `yaml:"vdTestDataDir"`
}

type VmConf struct {
	TestDataDir string `yaml:"testDataDir"`
}

type IpamConf struct {
	TestDataDir string `yaml:"testDataDir"`
}

type HelperImages struct {
	CurlImage string `yaml:"curlImage"`
}

func (c *Config) setEnvs() error {
	if e, ok := os.LookupEnv("E2E_NAMESPACE"); ok {
		c.Namespace = e
	}
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
		c.ClusterTransport.InsecureTls = v
	}
	// DisksConf
	if e, ok := os.LookupEnv("E2E_DISKS_UPLOADHELPERIMAGE"); ok {
		c.Disks.UploadHelperImage = e
	}
	if e, ok := os.LookupEnv("E2E_DISKS_CVITESTDATADIR"); ok {
		c.Disks.CviTestDataDir = e
	}
	if e, ok := os.LookupEnv("E2E_DISKS_VITESTDATADIR"); ok {
		c.Disks.ViTestDataDir = e
	}
	if e, ok := os.LookupEnv("E2E_DISKS_VDTESTDATADIR"); ok {
		c.Disks.VdTestDataDir = e
	}
	// VM
	if e, ok := os.LookupEnv("E2E_VM_TESTDATADIR"); ok {
		c.VM.TestDataDir = e
	}
	// IPAM
	if e, ok := os.LookupEnv("E2E_IPAM_TESTDATADIR"); ok {
		c.Ipam.TestDataDir = e
	}
	return nil
}

func (c *Config) GetTestCases() ([]string, error) {
	testDataValue := reflect.ValueOf(c.TestData)
	testDataType := reflect.TypeOf(c.TestData)
	excludedData := []string{"Sshkey", "SshUser"}
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

func GetNamePrefix() (string, error) {
	if prNumber, ok := os.LookupEnv("MODULES_MODULE_TAG"); ok && prNumber != "" {
		return prNumber, nil
	}

	res := git.GetHeadHash()
	if !res.WasSuccess() {
		return "", errors.New(res.StdErr())
	}

	commitHash := res.StdOut()
	commitHash = commitHash[:len(commitHash)-1]
	commitHash = fmt.Sprintf("head-%s", commitHash)
	return commitHash, nil
}

func (c *Config) SetNamespace(name string) {
	c.Namespace = name
}

func (k *Kustomize) SetParams(filePath, namespace, namePrefix string) error {
	var kustomizeFile Kustomize

	data, readErr := os.ReadFile(filePath)
	if readErr != nil {
		return readErr
	}

	unmarshalErr := yamlv3.Unmarshal([]byte(data), &kustomizeFile)
	if unmarshalErr != nil {
		return unmarshalErr
	}

	fileDir := filepath.Dir(filePath)
	testCaseName := filepath.Base(fileDir)

	kustomizeFile.Namespace = namespace + "-" + testCaseName
	kustomizeFile.NamePrefix = namePrefix + "-"
	kustomizeFile.Labels[0].Pairs["id"] = namePrefix
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

	unmarshalErr := yamlv3.Unmarshal([]byte(data), &kustomizeFile)
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

	unmarshalErr := yamlv3.Unmarshal([]byte(data), &kustomizeFile)
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

func GetModuleConfig() (*ModuleConfig, error) {
	res := kubectl.GetResource(kc.ResourceModuleConfig, "virtualization", kc.GetOptions{Output: "yaml"})
	if !res.WasSuccess() {
		return nil, errors.New(res.StdErr())
	}

	var mc ModuleConfig
	unmarshalErr := yamlv3.Unmarshal([]byte(res.StdOut()), &mc)
	if unmarshalErr != nil {
		return nil, unmarshalErr
	}
	return &mc, nil
}
