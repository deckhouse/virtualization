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
