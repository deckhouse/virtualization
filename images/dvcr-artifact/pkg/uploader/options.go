/*
Copyright 2026 Flant JSC

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

package uploader

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"

	ocpcrypto "github.com/openshift/library-go/pkg/crypto"
	"github.com/spf13/pflag"
	"kubevirt.io/containerized-data-importer/pkg/common"
	cryptowatch "kubevirt.io/containerized-data-importer/pkg/util/tls-crypto-watch"

	"github.com/deckhouse/virtualization-controller/dvcr-importers/pkg/auth"
	"github.com/deckhouse/virtualization-controller/dvcr-importers/pkg/registry"
)

const (
	// Environment variables. Destination-related names are kept unchanged for
	// backward compatibility with existing deployments.
	envListenAddress       = "LISTEN_ADDRESS"
	envListenPort          = "LISTEN_PORT"
	envHealthzPort         = "HEALTHZ_PORT"
	envServerCertFile      = "TLS_CERT_FILE"
	envServerKeyFile       = "TLS_KEY_FILE"
	envClientCAFile        = "CLIENT_CA_FILE"
	envClientName          = "CLIENT_NAME"
	envDestinationCABundle = "UPLOADER_DESTINATION_CA_BUNDLE"
	envChecksums           = "UPLOADER_CHECKSUMS"

	defaultListenAddress = "0.0.0.0"
	defaultListenPort    = 8444
)

// Options is the full command-line/environment configuration of the uploader.
// It knows how to register its flags and how to turn itself into a ready Server.
type Options struct {
	ListenAddress string
	ListenPort    int
	HealthzPort   int

	// Server TLS identity. When both CertFile and KeyFile are set the server
	// serves HTTPS, otherwise plain HTTP.
	CertFile string
	KeyFile  string
	// ClientCAFile enables mTLS: incoming client certificates are required and
	// verified against it. ClientName, when set, additionally restricts the
	// accepted client certificate CommonName.
	ClientCAFile  string
	ClientName    string
	Ciphers       string
	MinTLSVersion string

	// Destination DVCR registry.
	DestinationEndpoint   string
	DestinationUsername   string
	DestinationPassword   string
	DestinationAuthConfig string
	DestinationInsecure   bool
	DestinationCABundle   string

	// Checksums the uploaded data has to match, in the algorithm:sum format,
	// comma separated. Empty means the upload is accepted as it arrives.
	Checksums string
}

// AddFlags registers the uploader flags, each defaulting to its environment
// variable so existing deployments keep working without command-line arguments.
func (o *Options) AddFlags(fs *pflag.FlagSet) {
	fs.StringVar(&o.ListenAddress, "listen-address", envStr(envListenAddress, defaultListenAddress), "Address the upload server binds to")
	fs.IntVar(&o.ListenPort, "listen-port", envInt(envListenPort, defaultListenPort), "Port the upload server listens on")
	fs.IntVar(&o.HealthzPort, "healthz-port", envInt(envHealthzPort, defaultHealthzPort), "Port the healthz endpoint listens on")

	fs.StringVar(&o.CertFile, "server-cert", envStr(envServerCertFile, ""), "Path to the server TLS certificate; enables HTTPS when set together with --server-key")
	fs.StringVar(&o.KeyFile, "server-key", envStr(envServerKeyFile, ""), "Path to the server TLS private key; enables HTTPS when set together with --server-cert")
	fs.StringVar(&o.ClientCAFile, "client-ca", envStr(envClientCAFile, ""), "Path to the CA used to verify incoming client certificates; enables mTLS when set")
	fs.StringVar(&o.ClientName, "client-name", envStr(envClientName, ""), "Expected CommonName of the client certificate when mTLS is enabled")
	fs.StringVar(&o.Ciphers, "tls-ciphers", envStr(common.CiphersTLSVar, ""), "Comma-separated list of allowed TLS cipher suites")
	fs.StringVar(&o.MinTLSVersion, "tls-min-version", envStr(common.MinVersionTLSVar, ""), "Minimum supported TLS version")

	fs.StringVar(&o.DestinationEndpoint, "destination-endpoint", envStr(common.UploaderDestinationEndpoint, ""), "DVCR image endpoint to push the uploaded image to")
	fs.StringVar(&o.DestinationUsername, "destination-access-key-id", envStr(common.UploaderDestinationAccessKeyID, ""), "DVCR registry username")
	fs.StringVar(&o.DestinationPassword, "destination-secret-key", envStr(common.UploaderDestinationSecretKey, ""), "DVCR registry password")
	fs.StringVar(&o.DestinationAuthConfig, "destination-auth-config", envStr(common.UploaderDestinationAuthConfig, ""), "Path to a docker auth config used to resolve DVCR registry credentials")
	fs.BoolVar(&o.DestinationInsecure, "destination-insecure-tls", envBool(common.DestinationInsecureTLSVar, false), "Skip TLS verification of the DVCR registry certificate")
	fs.StringVar(&o.DestinationCABundle, "destination-ca-bundle", envStr(envDestinationCABundle, ""), "Path to a PEM file or a directory with PEM files used to verify the DVCR registry certificate")

	fs.StringVar(&o.Checksums, "checksums", envStr(envChecksums, ""), "Checksums the uploaded data has to match, in the algorithm:sum format, comma separated")
}

// Complete validates the options and turns them into a ready-to-run Server.
func (o *Options) Complete() (*Server, error) {
	if o.DestinationEndpoint == "" {
		return nil, fmt.Errorf("--destination-endpoint (%s) is required", common.UploaderDestinationEndpoint)
	}

	tlsConfig, err := o.buildTLSConfig()
	if err != nil {
		return nil, err
	}

	destination, err := o.buildDestination()
	if err != nil {
		return nil, err
	}

	checksums, err := registry.ParseChecksums(o.Checksums)
	if err != nil {
		return nil, fmt.Errorf("--checksums (%s): %w", envChecksums, err)
	}

	address := net.JoinHostPort(o.ListenAddress, strconv.Itoa(o.ListenPort))

	return NewServer(address, o.HealthzPort, tlsConfig, destination, checksums), nil
}

// buildTLSConfig assembles the server tls.Config from the TLS options. It returns
// nil (plain HTTP) when no server certificate is configured.
func (o *Options) buildTLSConfig() (*tls.Config, error) {
	if o.CertFile == "" && o.KeyFile == "" {
		if o.ClientCAFile != "" || o.ClientName != "" {
			return nil, fmt.Errorf("--client-ca/--client-name (mTLS) require --server-cert and --server-key")
		}
		return nil, nil
	}
	if o.CertFile == "" || o.KeyFile == "" {
		return nil, fmt.Errorf("--server-cert and --server-key must be set together")
	}
	if o.ClientName != "" && o.ClientCAFile == "" {
		return nil, fmt.Errorf("--client-name requires --client-ca")
	}

	cert, err := tls.LoadX509KeyPair(o.CertFile, o.KeyFile)
	if err != nil {
		return nil, fmt.Errorf("load server certificate: %w", err)
	}

	minVersion, err := o.minTLSVersion()
	if err != nil {
		return nil, err
	}

	cfg := &tls.Config{
		Certificates: []tls.Certificate{cert},
		MinVersion:   minVersion,
	}
	if o.Ciphers != "" {
		cfg.CipherSuites = cryptowatch.CipherSuitesIDs(strings.Split(o.Ciphers, ","))
	}

	if o.ClientCAFile != "" {
		clientCAs, err := loadCertPool(o.ClientCAFile)
		if err != nil {
			return nil, fmt.Errorf("load client CA: %w", err)
		}

		cfg.ClientCAs = clientCAs
		cfg.ClientAuth = tls.RequireAndVerifyClientCert

		if o.ClientName != "" {
			cfg.VerifyPeerCertificate = verifyClientCommonName(o.ClientName)
		}
	}

	return cfg, nil
}

func (o *Options) minTLSVersion() (uint16, error) {
	if o.MinTLSVersion == "" {
		return 0, nil
	}

	v, err := ocpcrypto.TLSVersion(o.MinTLSVersion)
	if err != nil {
		return 0, fmt.Errorf("invalid --tls-min-version %q: %w", o.MinTLSVersion, err)
	}

	return v, nil
}

// buildDestination resolves the DVCR credentials (directly or from the docker
// auth config) and returns the destination description.
func (o *Options) buildDestination() (Destination, error) {
	dst := Destination{
		Endpoint: o.DestinationEndpoint,
		Username: o.DestinationUsername,
		Password: o.DestinationPassword,
		Insecure: o.DestinationInsecure,
		CABundle: o.DestinationCABundle,
	}

	if dst.Username != "" || dst.Password != "" || o.DestinationAuthConfig == "" {
		return dst, nil
	}

	authFile, err := auth.RegistryAuthFile(o.DestinationAuthConfig)
	if err != nil {
		return Destination{}, fmt.Errorf("parse destination auth config: %w", err)
	}

	dst.Username, dst.Password, err = auth.CredsFromRegistryAuthFile(authFile, dst.Endpoint)
	if err != nil {
		return Destination{}, fmt.Errorf("get creds from destination auth config: %w", err)
	}

	return dst, nil
}

// verifyClientCommonName returns a tls.Config.VerifyPeerCertificate that accepts
// only a client certificate whose CommonName matches name.
func verifyClientCommonName(name string) func([][]byte, [][]*x509.Certificate) error {
	return func(_ [][]byte, verifiedChains [][]*x509.Certificate) error {
		var presented []string
		for _, chain := range verifiedChains {
			if len(chain) == 0 {
				continue
			}
			if chain[0].Subject.CommonName == name {
				return nil
			}
			presented = append(presented, chain[0].Subject.CommonName)
		}

		return fmt.Errorf("client certificate CommonName %q is not allowed, expected %q", strings.Join(presented, ", "), name)
	}
}

// loadCertPool reads PEM certificates from a file and returns a CertPool.
func loadCertPool(path string) (*x509.CertPool, error) {
	pemData, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read %q: %w", path, err)
	}

	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(pemData) {
		return nil, fmt.Errorf("no valid certificates found in %q", path)
	}

	return pool, nil
}
