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
	"fmt"
	"os"
	"slices"
	"strconv"

	gt "github.com/deckhouse/virtualization/tests/e2e/git"
	kc "github.com/deckhouse/virtualization/tests/e2e/kubectl"
	yamlv3 "gopkg.in/yaml.v3"
	"k8s.io/apimachinery/pkg/util/yaml"
)

var (
	conf    *Config
	err     error
	git     gt.Git
	kubectl kc.Kubectl
)

func init() {
	if conf, err = GetConfig(); err != nil {
		panic(err)
	}
	if git, err = gt.NewGit(); err != nil {
		panic(err)
	}
	if kubectl, err = kc.NewKubectl(kc.KubectlConf(conf.ClusterTransport)); err != nil {
		panic(err)
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

	if err := yaml.Unmarshal(data, &conf); err != nil {
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
	ApiVersion     string            `yaml:"apiVersion"`
	CommonLabels   map[string]string `yaml:"commonLabels"`
	Configurations []string          `yaml:"configurations"`
	Kind           string            `yaml:"kind"`
	Namespace      string            `yaml:"namespace"`
	NamePrefix     string            `yaml:"namePrefix"`
	Resources      []string          `yaml:"resources"`
}

type Config struct {
	Namespace               string           `yaml:"namespace"`
	ClusterTransport        ClusterTransport `yaml:"clusterTransport"`
	Disks                   DisksConf        `yaml:"disks"`
	VM                      VmConf           `yaml:"vm"`
	Ipam                    IpamConf         `yaml:"ipam"`
	HelperImages            HelperImages     `yaml:"helperImages"`
	VirtualizationResources string           `yaml:"virtualizationResources"`
	CustomSubnet            string           `yaml:"customSubnet"`
}

type ClusterTransport struct {
	KubeConfig           string `yaml:"kubeConfig"`
	Token                string `yaml:"token"`
	Endpoint             string `yaml:"endpoint"`
	CertificateAuthority string `yaml:"insecureTls"`
	InsecureTls          bool   `yaml:"certificateAuthority"`
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

type VirtualMachineIPAddress struct {
	ApiVersion string   `yaml:"apiVersion"`
	Kind       string   `yaml:"kind"`
	Metadata   Metadata `yaml:"metadata"`
	Spec       VMIPSpec `yaml:"spec"`
}

type VMIPSpec struct {
	Type     string `yaml:"type"`
	StaticIP string `yaml:"staticIP,omitempty"`
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

func GetNamePrefix() (string, error) {
	if prNumber, ok := os.LookupEnv("MODULES_MODULE_TAG"); ok && prNumber != "" {
		return prNumber, nil
	}

	res := git.GetHeadHash()
	if !res.WasSuccess() {
		return "", fmt.Errorf(res.StdErr())
	}

	commitHash := res.StdOut()
	commitHash = commitHash[:len(commitHash)-1]
	return commitHash, nil
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

	kustomizeFile.Namespace = namespace
	kustomizeFile.NamePrefix = namePrefix + "-"
	kustomizeFile.CommonLabels["testcase"] = namePrefix
	updatedKustomizeFile, marshalErr := yamlv3.Marshal(&kustomizeFile)
	if marshalErr != nil {
		return marshalErr
	}

	writeErr := os.WriteFile(filePath, updatedKustomizeFile, 0644)
	if writeErr != nil {
		return writeErr
	}

	return nil
}

func CheckCustomSubnet(mc *ModuleConfig, subnet string) bool {
	return slices.Contains(mc.Spec.Settings.VirtualMachineCIDRs, subnet)
}

func SetupCustomSubnet(mc *ModuleConfig, subnet string) error {
	subnetConfigured := CheckCustomSubnet(mc, subnet)
	if subnetConfigured {
		return nil
	}

	mc.Spec.Settings.VirtualMachineCIDRs = append(mc.Spec.Settings.VirtualMachineCIDRs, conf.CustomSubnet)
	updatedModuleConfig, err := yamlv3.Marshal(mc)
	if err != nil {
		return err
	}

	filePath := "/tmp/mc.yaml"
	writeErr := os.WriteFile(filePath, updatedModuleConfig, 0644)
	if writeErr != nil {
		return writeErr
	}

	res := kubectl.Apply(filePath, kc.ApplyOptions{})
	if !res.WasSuccess() {
		return fmt.Errorf(res.StdErr())
	}

	return nil
}

func GetModuleConfig() (*ModuleConfig, error) {
	moduleConfig := kc.Resource("mc")
	res := kubectl.GetResource(moduleConfig, "virtualization", kc.GetOptions{Output: "yaml"})
	if !res.WasSuccess() {
		return nil, fmt.Errorf(res.StdErr())
	}

	var mc ModuleConfig
	unmarshalErr := yamlv3.Unmarshal([]byte(res.StdOut()), &mc)
	if unmarshalErr != nil {
		return nil, unmarshalErr
	}
	return &mc, nil
}
