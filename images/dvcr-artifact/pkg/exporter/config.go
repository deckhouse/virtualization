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
	ListenAddress  string
	ListenPort     int
	CertPath       string
	KeyPath        string
	Image          string
	AuthConfigPath string
	Insecure       bool
}

func (c *Config) Load(fs *pflag.FlagSet) {
	fs.StringVar(&c.ListenAddress, "listen-address", "0.0.0.0", "Listen address")
	fs.IntVar(&c.ListenPort, "listen-port", 8444, "Listen port")
	fs.StringVar(&c.CertPath, "cert-path", os.Getenv("EXPORTER_CERT_PATH"), "Path to TLS certificate")
	fs.StringVar(&c.KeyPath, "key-path", os.Getenv("EXPORTER_KEY_PATH"), "Path to TLS key")
	fs.StringVar(&c.Image, "image", os.Getenv("EXPORTER_IMAGE"), "Image")
	fs.StringVar(&c.AuthConfigPath, "auth-config-path", os.Getenv("EXPORTER_AUTH_CONFIG"), "Path to auth config file")
	fs.BoolVar(&c.Insecure, "insecure", strings.ToLower(os.Getenv("EXPORTER_INSECURE")) == "true", "Insecure")
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
	if c.AuthConfigPath != "" {
		authFile, err := auth.RegistryAuthFile(c.AuthConfigPath)
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
		WithInsecure(c.Insecure),
	), nil
}

type Exporter interface {
	Run(ctx context.Context) error
}
