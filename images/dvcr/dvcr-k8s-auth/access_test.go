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

//go:build dvcr_registry

// Integration tests for verifyJWT: mint tokens as the controller (golang-jwt) and
// as an x5c attacker (go-jose), verify through the real distribution backend. The
// only end-to-end check of the empty-Roots x5c defense.

package dvcrk8s

import (
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"testing"
	"time"

	golangjwt "github.com/golang-jwt/jwt/v5"

	jose "github.com/go-jose/go-jose/v4"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

func TestAccessController(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "DVCR Auth Suite")
}

const (
	testIssuer   = "virtualization-controller"
	testAudience = "dvcr"
	testKeyID    = "dvcr"
)

func newController(pub crypto.PublicKey) *accessController {
	return &accessController{
		realm:       "dvcr",
		jwtIssuer:   testIssuer,
		jwtAudience: testAudience,
		trustedKeys: map[string]crypto.PublicKey{testKeyID: pub},
	}
}

func testClaims(now time.Time) map[string]any {
	return map[string]any{
		"iss": testIssuer,
		"aud": testAudience,
		"iat": now.Unix(),
		"nbf": now.Add(-30 * time.Second).Unix(),
		"exp": now.Add(time.Hour).Unix(),
		"access": []map[string]any{
			{"type": "repository", "name": "cvi/img", "actions": []string{"pull", "push"}},
		},
	}
}

// mintKidToken signs a token the way the controller does: golang-jwt/v5, ES256,
// kid header, no embedded key material.
func mintKidToken(key *ecdsa.PrivateKey, claims map[string]any) string {
	GinkgoHelper()
	tok := golangjwt.NewWithClaims(golangjwt.SigningMethodES256, golangjwt.MapClaims(claims))
	tok.Header["kid"] = testKeyID
	raw, err := tok.SignedString(key)
	Expect(err).NotTo(HaveOccurred())
	return raw
}

// mintX5cToken forges a token carrying its own self-signed certificate in the
// x5c header — the attack the empty-Roots pool must defeat.
func mintX5cToken(claims map[string]any) string {
	GinkgoHelper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	Expect(err).NotTo(HaveOccurred())
	tmpl := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "attacker"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
	}
	certDER, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	Expect(err).NotTo(HaveOccurred())
	opts := (&jose.SignerOptions{}).WithHeader("x5c", []string{base64.StdEncoding.EncodeToString(certDER)})
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.ES256, Key: key}, opts)
	Expect(err).NotTo(HaveOccurred())
	payload, err := json.Marshal(claims)
	Expect(err).NotTo(HaveOccurred())
	jws, err := signer.Sign(payload)
	Expect(err).NotTo(HaveOccurred())
	raw, err := jws.CompactSerialize()
	Expect(err).NotTo(HaveOccurred())
	return raw
}

func newKey() *ecdsa.PrivateKey {
	GinkgoHelper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	Expect(err).NotTo(HaveOccurred())
	return key
}

var _ = Describe("verifyJWT", func() {
	It("accepts a valid kid-signed token and returns its access grants", func() {
		key := newKey()
		ac := newController(&key.PublicKey)

		grants, err := ac.verifyJWT(mintKidToken(key, testClaims(time.Now())))
		Expect(err).NotTo(HaveOccurred())
		Expect(grants).To(HaveLen(1))
		Expect(grants[0].Name).To(Equal("cvi/img"))
		Expect(grants[0].Type).To(Equal("repository"))
	})

	// The core security guarantee: a token whose signing key is smuggled in via
	// x5c must not be trusted, even though its self-signed cert is internally valid.
	It("rejects an x5c-smuggled token", func() {
		key := newKey()
		ac := newController(&key.PublicKey)

		_, err := ac.verifyJWT(mintX5cToken(testClaims(time.Now())))
		Expect(err).To(HaveOccurred())
	})

	DescribeTable("rejects an invalid token",
		func(makeToken func(key *ecdsa.PrivateKey) string) {
			key := newKey()
			ac := newController(&key.PublicKey)

			_, err := ac.verifyJWT(makeToken(key))
			Expect(err).To(HaveOccurred())
		},
		Entry("expired", func(key *ecdsa.PrivateKey) string {
			claims := testClaims(time.Now())
			claims["exp"] = time.Now().Add(-time.Hour).Unix()
			return mintKidToken(key, claims)
		}),
		Entry("wrong issuer", func(key *ecdsa.PrivateKey) string {
			claims := testClaims(time.Now())
			claims["iss"] = "someone-else"
			return mintKidToken(key, claims)
		}),
		Entry("wrong audience", func(key *ecdsa.PrivateKey) string {
			claims := testClaims(time.Now())
			claims["aud"] = "not-dvcr"
			return mintKidToken(key, claims)
		}),
		Entry("untrusted signing key", func(*ecdsa.PrivateKey) string {
			return mintKidToken(newKey(), testClaims(time.Now()))
		}),
		Entry("tampered payload", func(key *ecdsa.PrivateKey) string {
			raw := mintKidToken(key, testClaims(time.Now()))
			return raw[:len(raw)-3] + "AAA"
		}),
	)
})

var _ = Describe("classify", func() {
	It("maps static credentials and scoped tokens to roles", func() {
		key := newKey()
		ac := newController(&key.PublicKey)
		ac.adminUsername = "admin"
		ac.adminPassword = []byte("admin-pass")
		ac.pullerUsername = "node-puller"
		ac.pullerPassword = []byte("puller-pass")

		s, _, err := ac.classify("admin", "admin-pass")
		Expect(err).NotTo(HaveOccurred())
		Expect(s.Role).To(Equal(RoleAdmin))

		s, _, err = ac.classify("node-puller", "puller-pass")
		Expect(err).NotTo(HaveOccurred())
		Expect(s.Role).To(Equal(RolePuller))

		// A scoped token presented under the admin username must not become admin.
		scoped := mintKidToken(key, testClaims(time.Now()))
		s, _, err = ac.classify("admin", scoped)
		Expect(err).NotTo(HaveOccurred())
		Expect(s.Role).To(Equal(RoleScoped))
	})
})
