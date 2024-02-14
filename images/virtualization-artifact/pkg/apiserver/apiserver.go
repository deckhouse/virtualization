package apiserver

import (
	"errors"
	"fmt"
	"github.com/deckhouse/virtualization-controller/pkg/common"
	"os"
	"sync"
)

var (
	Once sync.Once
	conf *Config
)

var ErrConfigInvalid = errors.New("configuration is invalid")

type Config struct {
	KVApiServerEndpoint  string
	KVApiServerCertsPath string
	CertsPath            string
}

func (c *Config) Validate() (bool, error) {
	if c.KVApiServerEndpoint == "" {
		return false, fmt.Errorf("KVApiServerEndpoint is required. %w", ErrConfigInvalid)
	}
	if c.KVApiServerCertsPath == "" {
		return false, fmt.Errorf("KVApiServerCertsPath is required. %w", ErrConfigInvalid)
	}
	if c.CertsPath == "" {
		return false, fmt.Errorf("CertsPath is required. %w", ErrConfigInvalid)
	}
	return true, nil
}

func GetConf() (*Config, error) {
	Once.Do(func() {
		conf.KVApiServerEndpoint = os.Getenv(common.KubevirtAPIServerEndpointVar)
		conf.KVApiServerCertsPath = os.Getenv(common.KubevirtAPIServerCertsPathVar)
		conf.CertsPath = os.Getenv(common.VirtualizationApiCertsPathVar)
	})
	if val, err := conf.Validate(); !val {
		return nil, err
	}
	return conf, nil
}
