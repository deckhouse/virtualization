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

package registrytoken

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestRegistryToken(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "RegistryToken Suite")
}

// newSignerPEM returns a fresh PKCS8-encoded ECDSA private key and its public key.
func newSignerPEM() ([]byte, *ecdsa.PublicKey) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	Expect(err).NotTo(HaveOccurred())
	der, err := x509.MarshalPKCS8PrivateKey(key)
	Expect(err).NotTo(HaveOccurred())
	return pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der}), &key.PublicKey
}

var _ = Describe("Signer", func() {
	now := time.Unix(1_700_000_000, 0)

	It("mints a token the registry accepts, with the expected claims", func() {
		keyPEM, pub := newSignerPEM()
		signer, err := NewSignerFromPEM(keyPEM)
		Expect(err).NotTo(HaveOccurred())

		access := []Access{{Type: "repository", Name: "cvi/my-image", Actions: []string{"pull", "push"}}}
		raw, err := signer.Sign(access, now)
		Expect(err).NotTo(HaveOccurred())

		// Verify signature and claims the way the registry will.
		parsed, err := jwt.Parse(raw, func(*jwt.Token) (any, error) { return pub, nil },
			jwt.WithValidMethods([]string{"ES256"}),
			jwt.WithTimeFunc(func() time.Time { return now.Add(time.Minute) }))
		Expect(err).NotTo(HaveOccurred())

		Expect(parsed.Header["kid"]).To(Equal(KeyID))
		claims := parsed.Claims.(jwt.MapClaims)
		Expect(claims["iss"]).To(Equal(Issuer))
		Expect(claims["aud"]).To(Equal(Audience))

		acc, ok := claims["access"].([]any)
		Expect(ok).To(BeTrue())
		Expect(acc).To(HaveLen(1))
		entry := acc[0].(map[string]any)
		Expect(entry["name"]).To(Equal("cvi/my-image"))
		Expect(entry["type"]).To(Equal("repository"))

		// Time claims must frame exactly [iat-30s, iat+DefaultTTL] so the registry
		// accepts a token immediately and rejects it past its lifetime.
		Expect(int64(claims["iat"].(float64))).To(Equal(now.Unix()))
		Expect(int64(claims["nbf"].(float64))).To(Equal(now.Add(-30 * time.Second).Unix()))
		Expect(int64(claims["exp"].(float64))).To(Equal(now.Add(DefaultTTL).Unix()))
	})

	It("binds the token to its signing key: a wrong public key must not verify", func() {
		keyPEM, _ := newSignerPEM()
		signer, err := NewSignerFromPEM(keyPEM)
		Expect(err).NotTo(HaveOccurred())
		_, otherPub := newSignerPEM()

		raw, err := signer.Sign(nil, now)
		Expect(err).NotTo(HaveOccurred())

		_, err = jwt.Parse(raw, func(*jwt.Token) (any, error) { return otherPub, nil },
			jwt.WithValidMethods([]string{"ES256"}),
			jwt.WithTimeFunc(func() time.Time { return now.Add(time.Minute) }))
		Expect(err).To(HaveOccurred())
	})

	It("mints a token that no longer verifies past DefaultTTL", func() {
		keyPEM, pub := newSignerPEM()
		signer, err := NewSignerFromPEM(keyPEM)
		Expect(err).NotTo(HaveOccurred())

		raw, err := signer.Sign(nil, now)
		Expect(err).NotTo(HaveOccurred())

		_, err = jwt.Parse(raw, func(*jwt.Token) (any, error) { return pub, nil },
			jwt.WithTimeFunc(func() time.Time { return now.Add(DefaultTTL + time.Hour) }))
		Expect(err).To(HaveOccurred())
	})

	// Key-parsing branches: both accepted EC formats (PKCS8, SEC1) and the
	// rejected inputs (non-PEM bytes, a non-ECDSA key).
	DescribeTable("NewSignerFromPEM",
		func(pemFn func() []byte, wantErr bool) {
			signer, err := NewSignerFromPEM(pemFn())
			if wantErr {
				Expect(err).To(HaveOccurred())
				return
			}
			Expect(err).NotTo(HaveOccurred())
			_, err = signer.Sign(nil, now)
			Expect(err).NotTo(HaveOccurred())
		},
		Entry("PKCS8 EC key", func() []byte {
			key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
			Expect(err).NotTo(HaveOccurred())
			der, err := x509.MarshalPKCS8PrivateKey(key)
			Expect(err).NotTo(HaveOccurred())
			return pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})
		}, false),
		Entry("SEC1 EC key", func() []byte {
			key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
			Expect(err).NotTo(HaveOccurred())
			der, err := x509.MarshalECPrivateKey(key)
			Expect(err).NotTo(HaveOccurred())
			return pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: der})
		}, false),
		Entry("not a PEM block", func() []byte {
			return []byte("definitely not pem")
		}, true),
		Entry("RSA key is not ECDSA", func() []byte {
			key, err := rsa.GenerateKey(rand.Reader, 2048)
			Expect(err).NotTo(HaveOccurred())
			der, err := x509.MarshalPKCS8PrivateKey(key)
			Expect(err).NotTo(HaveOccurred())
			return pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})
		}, true),
	)
})
