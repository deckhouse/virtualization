/*
Copyright 2025 Flant JSC

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

package exporter

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/pflag"

	"github.com/deckhouse/virtualization-controller/dvcr-importers/pkg/auth"
)

type Config struct {
	ListenAddress      string
	ListenPort         int
	CertPath           string
	KeyPath            string
	Image              string
	DestAuthConfigPath string
	DestCertPath       string
	DestInsecure       bool
}

func (c *Config) Load(fs *pflag.FlagSet) {
	fs.StringVar(&c.ListenAddress, "listen-address", "0.0.0.0", "Listen address")
	fs.IntVar(&c.ListenPort, "listen-port", 8444, "Listen port")
	fs.StringVar(&c.CertPath, "cert-path", os.Getenv("EXPORTER_CERT_PATH"), "Path to TLS certificate")
	fs.StringVar(&c.KeyPath, "key-path", os.Getenv("EXPORTER_KEY_PATH"), "Path to TLS key")
	fs.StringVar(&c.Image, "image", os.Getenv("EXPORTER_IMAGE"), "Image")
	fs.StringVar(&c.DestAuthConfigPath, "dest-auth-config-path", os.Getenv("EXPORTER_DEST_AUTH_CONFIG"), "Path to auth config file")
	fs.StringVar(&c.DestCertPath, "dest-cert-path", os.Getenv("EXPORTER_DEST_CERT_PATH"), "Path to destination TLS certificate")
	fs.BoolVar(&c.DestInsecure, "dest-insecure", strings.ToLower(os.Getenv("EXPORTER_DEST_INSECURE")) == "true", "DestInsecure")
}

func (c *Config) Validate() error {
	if c.ListenAddress == "" {
		return fmt.Errorf("ListenAddress is required")
	}

	if c.ListenPort == 0 {
		return fmt.Errorf("ListenPort is required")
	}

	if c.Image == "" {
		return fmt.Errorf("Image is required")
	}
	return nil
}

func (c *Config) Complete() (Exporter, error) {
	var (
		username string
		password string
	)
	if c.DestAuthConfigPath != "" {
		authFile, err := auth.RegistryAuthFile(c.DestAuthConfigPath)
		if err != nil {
			return nil, fmt.Errorf("error parsing destination auth config: %w", err)
		}

		username, password, err = auth.CredsFromRegistryAuthFile(authFile, c.Image)
		if err != nil {
			return nil, fmt.Errorf("error getting creds from destination auth config: %w", err)
		}
	}

	return NewExportServer(c.Image, c.ListenAddress, c.ListenPort,
		WithTLS(c.CertPath, c.KeyPath),
		WithAuth(username, password),
		WithDestInsecure(c.DestInsecure),
		WithDestCert(c.DestCertPath),
	), nil
}

type Exporter interface {
	Run(ctx context.Context) error
}
