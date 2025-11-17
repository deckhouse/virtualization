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

package dvcr

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"net/http"

	"github.com/google/go-containerregistry/pkg/authn/kubernetes"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/remote/transport"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ImageChecker provides functionality to check if images exist in a registry.
type ImageChecker interface {
	CheckImageExists(ctx context.Context, imageURL string) (bool, error)
}

// DefaultImageChecker implements ImageChecker using go-containerregistry.
type DefaultImageChecker struct {
	client       client.Client
	dvcrSettings *Settings
}

// NewImageChecker creates a new ImageChecker with the provided client and DVCR settings.
func NewImageChecker(client client.Client, dvcrSettings *Settings) ImageChecker {
	return &DefaultImageChecker{
		client:       client,
		dvcrSettings: dvcrSettings,
	}
}

// CheckImageExists checks if an image exists in the registry by performing a lightweight HEAD request.
// It returns true if the image exists, false if it doesn't exist, and an error for other failures.
func (c *DefaultImageChecker) CheckImageExists(ctx context.Context, imageURL string) (bool, error) {
	if imageURL == "" {
		return false, fmt.Errorf("image URL is empty")
	}

	ref, err := name.ParseReference(imageURL)
	if err != nil {
		return false, fmt.Errorf("failed to parse image reference %q: %w", imageURL, err)
	}

	opts, err := c.remoteOptions(ctx)
	if err != nil {
		return false, err
	}

	_, err = remote.Head(ref, opts...)
	if err != nil {
		var transportErr *transport.Error
		if errors.As(err, &transportErr) && transportErr.StatusCode == http.StatusNotFound {
			return false, nil
		}

		return false, fmt.Errorf("failed to check image existence for %q: %w", imageURL, err)
	}

	return true, nil
}

// remoteOptions returns the remote options for registry operations.
func (c *DefaultImageChecker) remoteOptions(ctx context.Context) ([]remote.Option, error) {
	opts := []remote.Option{
		remote.WithContext(ctx),
	}

	// Fetch authentication credentials from Secret if configured
	if c.dvcrSettings.AuthSecret != "" {
		keychain, err := kubernetes.NewInCluster(ctx, kubernetes.Options{
			Namespace:        c.dvcrSettings.AuthSecretNamespace,
			ImagePullSecrets: []string{c.dvcrSettings.AuthSecret},
		})
		if err != nil {
			return nil, fmt.Errorf("failed to create keychain: %w", err)
		}

		opts = append(opts, remote.WithAuthFromKeychain(keychain))
	}

	// Fetch CA certificate from Secret if configured
	var caCert []byte
	if c.dvcrSettings.CertsSecret != "" {
		secret := &corev1.Secret{}
		err := c.client.Get(ctx, types.NamespacedName{
			Name:      c.dvcrSettings.CertsSecret,
			Namespace: c.dvcrSettings.CertsSecretNamespace,
		}, secret)
		if err != nil {
			return nil, fmt.Errorf("failed to get certs secret %s/%s: %w",
				c.dvcrSettings.CertsSecretNamespace, c.dvcrSettings.CertsSecret, err)
		}

		var ok bool
		caCert, ok = secret.Data["ca.crt"]
		if !ok {
			return nil, fmt.Errorf("ca.crt not found in secret %s/%s",
				c.dvcrSettings.CertsSecretNamespace, c.dvcrSettings.CertsSecret)
		}
	}

	// Configure TLS based on CA certificate or insecure flag
	var tlsConfig *tls.Config
	if len(caCert) > 0 {
		certPool := x509.NewCertPool()
		if !certPool.AppendCertsFromPEM(caCert) {
			return nil, fmt.Errorf("failed to append CA certificate to pool")
		}
		tlsConfig = &tls.Config{
			RootCAs: certPool,
		}
	} else if c.dvcrSettings.InsecureTLS == "true" {
		tlsConfig = &tls.Config{
			InsecureSkipVerify: true,
		}
	}

	if tlsConfig != nil {
		transport := http.DefaultTransport.(*http.Transport).Clone()
		transport.TLSClientConfig = tlsConfig
		opts = append(opts, remote.WithTransport(transport))
	}

	return opts, nil
}
