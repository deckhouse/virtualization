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

package usbip

import (
	"crypto/tls"
	"crypto/x509"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/deckhouse/virtualization-dra/pkg/usb"
)

type ClientAuthType tls.ClientAuthType

func (c *ClientAuthType) String() string {
	cc := tls.ClientAuthType(*c)
	return cc.String()
}

func (c *ClientAuthType) Set(s string) error {
	switch s {
	case "NoClientCert":
		*c = ClientAuthType(tls.NoClientCert)
	case "RequestClientCert":
		*c = ClientAuthType(tls.RequestClientCert)
	case "RequireAnyClientCert":
		*c = ClientAuthType(tls.RequireAnyClientCert)
	case "VerifyClientCertIfGiven":
		*c = ClientAuthType(tls.VerifyClientCertIfGiven)
	case "RequireAndVerifyClientCert":
		*c = ClientAuthType(tls.RequireAndVerifyClientCert)
	default:
		return fmt.Errorf("invalid client auth type: %s", s)
	}
	return nil
}

type USBIPDConfig struct {
	ServerCertificateFile string
	ServerKeyFile         string

	RootCAFile string

	ClientCAFile       string
	ClientAuthType     *ClientAuthType
	clientAuthType     int
	InsecureSkipVerify bool

	Port                    int
	GracefulShutdownTimeout time.Duration

	Monitor *usb.Monitor
}

func (c *USBIPDConfig) AddFlags(fs *flag.FlagSet) {
	fs.IntVar(&c.Port, "usbipd-port", 0, "USBIPD port")
	fs.StringVar(&c.ServerCertificateFile, "usbipd-server-certificate-file", "", "USBIPD server certificate file")
	fs.StringVar(&c.ServerKeyFile, "usbipd-server-key-file", "", "USBIPD server key file")
	fs.StringVar(&c.RootCAFile, "usbipd-root-ca-file", "", "USBIPD root CA file")
	fs.StringVar(&c.ClientCAFile, "usbipd-client-ca-file", "", "USBIPD client CA file")
	fs.Var(c.ClientAuthType, "usbipd-client-auth-type", "USBIPD client auth type")
	fs.BoolVar(&c.InsecureSkipVerify, "usbipd-insecure-skip-verify", false, "USBIPD insecure skip verify")
	fs.DurationVar(&c.GracefulShutdownTimeout, "usbipd-graceful-shutdown-timeout", 0, "USBIPD graceful shutdown timeout")
}

func (c *USBIPDConfig) Validate() error {
	if c.Port == 0 {
		return fmt.Errorf("port is required")
	}

	if c.ServerCertificateFile != "" && c.ServerKeyFile == "" {
		return fmt.Errorf("server key file is required if server certificate file is provided")
	}

	if c.ServerCertificateFile == "" && c.ServerKeyFile != "" {
		return fmt.Errorf("server certificate file is required if server key file is provided")
	}

	if c.Monitor == nil {
		return fmt.Errorf("monitor is required")
	}

	return nil
}

func (c *USBIPDConfig) Complete() (*USBIPD, error) {
	var opts []Option
	if c.GracefulShutdownTimeout != 0 {
		opts = append(opts, WithGracefulShutdownTimeout(c.GracefulShutdownTimeout))
	}

	var serverCertificate *tls.Certificate
	if c.ServerCertificateFile != "" && c.ServerKeyFile != "" {
		certificate, err := tls.LoadX509KeyPair(c.ServerCertificateFile, c.ServerKeyFile)
		if err != nil {
			return nil, err
		}
		serverCertificate = &certificate
	}

	rootCACertPool, err := loadCAPoolFromFile(c.RootCAFile)
	if err != nil {
		return nil, err
	}

	clientCACertPool, err := loadCAPoolFromFile(c.ClientCAFile)
	if err != nil {
		return nil, err
	}

	if serverCertificate != nil || rootCACertPool != nil || clientCACertPool != nil {
		tlsConfig := &tls.Config{
			RootCAs:            rootCACertPool,
			ClientCAs:          clientCACertPool,
			InsecureSkipVerify: c.InsecureSkipVerify,
		}
		if serverCertificate != nil {
			tlsConfig.Certificates = []tls.Certificate{*serverCertificate}
		}
		if c.ClientAuthType != nil {
			tlsConfig.ClientAuth = tls.ClientAuthType(*c.ClientAuthType)
		}

		opts = append(opts, WithTLSConfig(tlsConfig))
	}

	return NewUSBIPD(c.Port, c.Monitor, opts...), nil

}

func loadCAPoolFromFile(file string) (*x509.CertPool, error) {
	if file == "" {
		return nil, nil
	}

	caCertPool := x509.NewCertPool()
	caCertPEMBlock, err := os.ReadFile(file)
	if err != nil {
		return nil, fmt.Errorf("failed to read CA certificate: %w", err)
	}

	if !caCertPool.AppendCertsFromPEM(caCertPEMBlock) {
		return nil, fmt.Errorf("failed to parse CA certificate")
	}

	return caCertPool, nil
}
