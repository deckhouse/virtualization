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

package tls

import (
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"os"
	"strings"

	ocpcrypto "github.com/openshift/library-go/pkg/crypto"
	"kubevirt.io/containerized-data-importer/pkg/common"
	cryptowatch "kubevirt.io/containerized-data-importer/pkg/util/tls-crypto-watch"
)

func NewCertPool(certPath string) (*x509.CertPool, error) {
	cert, err := os.ReadFile(certPath)
	if err != nil {
		return nil, err
	}
	caCertPool := x509.NewCertPool()
	if ok := caCertPool.AppendCertsFromPEM(cert); !ok {
		return nil, fmt.Errorf("invalid ca cert file %s", cert)
	}
	return caCertPool, nil
}

func GetCryptoConfig() cryptowatch.CryptoConfig {
	ciphersNames := strings.Split(os.Getenv(common.CiphersTLSVar), ",")
	ciphers := cryptowatch.CipherSuitesIDs(ciphersNames)
	minTLSVersion, _ := ocpcrypto.TLSVersion(os.Getenv(common.MinVersionTLSVar))

	return cryptowatch.CryptoConfig{
		CipherSuites: ciphers,
		MinVersion:   minTLSVersion,
	}
}

func NewBuilder() *Builder {
	return &Builder{
		config: &tls.Config{},
	}
}

type Builder struct {
	config *tls.Config
}

func (b *Builder) WithClientCAs(certPool *x509.CertPool) *Builder {
	cryptoConfig := GetCryptoConfig()
	b.config.ClientCAs = certPool
	b.config.ClientAuth = tls.RequireAndVerifyClientCert
	b.config.MinVersion = cryptoConfig.MinVersion
	return b
}

func (b *Builder) WithRootCAs(certPool *x509.CertPool) *Builder {
	b.config.RootCAs = certPool
	return b
}

func (b *Builder) Build() *tls.Config {
	return b.config.Clone()
}
