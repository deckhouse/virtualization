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
	"k8s.io/apimachinery/pkg/util/yaml"
	"os"
	"strconv"
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

	if err := yaml.Unmarshal(data, &conf); err != nil {
		return nil, err
	}
	if err := conf.setEnvs(); err != nil {
		return nil, err
	}
	return &conf, nil
}

type Config struct {
	Namespace        string           `yaml:"namespace"`
	ClusterTransport ClusterTransport `yaml:"clusterTransport"`
	Disks            DisksConf        `yaml:"disks"`
	VM               VmConf           `yaml:"vm"`
	Ipam             IpamConf         `yaml:"ipam"`
	HelperImages     HelperImages     `yaml:"helperimages"`
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
	CvmiTestDataDir   string `yaml:"cvmiTestDataDir"`
	VmiTestDataDir    string `yaml:"vmiTestDataDir"`
	VmdTestDataDir    string `yaml:"vmdTestDataDir"`
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
	if e, ok := os.LookupEnv("E2E_DISKS_CVMITESTDATADIR"); ok {
		c.Disks.CvmiTestDataDir = e
	}
	if e, ok := os.LookupEnv("E2E_DISKS_VMITESTDATADIR"); ok {
		c.Disks.VmiTestDataDir = e
	}
	if e, ok := os.LookupEnv("E2E_DISKS_VMDTESTDATADIR"); ok {
		c.Disks.VmdTestDataDir = e
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
