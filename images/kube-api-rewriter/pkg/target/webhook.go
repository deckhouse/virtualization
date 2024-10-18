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

package target

import (
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"time"

	"github.com/deckhouse/kube-api-rewriter/pkg/tls/certmanager"
	"github.com/deckhouse/kube-api-rewriter/pkg/tls/certmanager/filesystem"
)

type Webhook struct {
	Client      *http.Client
	URL         *url.URL
	CertManager certmanager.CertificateManager
}

const (
	WebhookAddressVar    = "WEBHOOK_ADDRESS"
	WebhookServerNameVar = "WEBHOOK_SERVER_NAME"
	WebhookCertFileVar   = "WEBHOOK_CERT_FILE"
	WebhookKeyFileVar    = "WEBHOOK_KEY_FILE"
)

var (
	defaultWebhookTimeout = 30 * time.Second
	defaultWebhookAddress = "https://127.0.0.1:9443"
)

func NewWebhookTarget() (*Webhook, error) {
	var err error
	webhook := &Webhook{}

	// Target address and serverName.
	address := os.Getenv(WebhookAddressVar)
	if address == "" {
		address = defaultWebhookAddress
	}

	serverName := os.Getenv(WebhookServerNameVar)
	if serverName == "" {
		serverName = address
	}

	webhook.URL, err = url.Parse(address)
	if err != nil {
		return nil, err
	}

	// Certificate settings.
	certFile := os.Getenv(WebhookCertFileVar)
	keyFile := os.Getenv(WebhookKeyFileVar)
	if certFile == "" && keyFile != "" {
		return nil, fmt.Errorf("should specify cert file in %s if %s is not empty", WebhookCertFileVar, WebhookKeyFileVar)
	}
	if certFile != "" && keyFile == "" {
		return nil, fmt.Errorf("should specify key file in %s if %s is not empty", WebhookKeyFileVar, WebhookCertFileVar)
	}
	if certFile != "" && keyFile != "" {
		webhook.CertManager = filesystem.NewFileCertificateManager(certFile, keyFile)
	}

	// Construct TLS client without validation to connect to the local webhook server.
	dialer := &net.Dialer{
		Timeout: defaultWebhookTimeout,
	}

	tr := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true,
			ServerName:         serverName,
		},
		DisableKeepAlives:     true,
		IdleConnTimeout:       5 * time.Minute,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		DialContext:           dialer.DialContext,
	}

	webhook.Client = &http.Client{
		Transport: tr,
		Timeout:   defaultWebhookTimeout,
	}

	return webhook, nil
}
