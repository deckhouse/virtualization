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

package service

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/deckhouse/deckhouse/pkg/log"
	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
)

// genCert produces a self-signed certificate (also usable as its own CA) valid
// for the given hosts (IPs vs DNS names are detected automatically).
func genCert(hosts ...string) (tls.Certificate, []byte) {
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	Expect(err).NotTo(HaveOccurred())

	tmpl := x509.Certificate{
		SerialNumber:          big.NewInt(1),
		Subject:               pkix.Name{CommonName: hosts[0]},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	for _, h := range hosts {
		if ip := net.ParseIP(h); ip != nil {
			tmpl.IPAddresses = append(tmpl.IPAddresses, ip)
		} else {
			tmpl.DNSNames = append(tmpl.DNSNames, h)
		}
	}

	der, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, &priv.PublicKey, priv)
	Expect(err).NotTo(HaveOccurred())
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})

	keyDER, err := x509.MarshalECPrivateKey(priv)
	Expect(err).NotTo(HaveOccurred())
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})

	tlsCert, err := tls.X509KeyPair(certPEM, keyPEM)
	Expect(err).NotTo(HaveOccurred())
	return tlsCert, certPEM
}

// tlsServer starts an HTTPS test server presenting cert and replying with status.
func tlsServer(cert tls.Certificate, status int) *httptest.Server {
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(status)
	}))
	srv.TLS = &tls.Config{Certificates: []tls.Certificate{cert}}
	srv.StartTLS()
	return srv
}

func readyPod(ready bool) *corev1.Pod {
	status := corev1.ConditionFalse
	if ready {
		status = corev1.ConditionTrue
	}
	return &corev1.Pod{
		Status: corev1.PodStatus{
			Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: status}},
		},
	}
}

func ingWithURL(url string) *netv1.Ingress {
	return &netv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{annotations.AnnUploadURL: url}},
	}
}

func tlsSecret(certPEM []byte) *corev1.Secret {
	return &corev1.Secret{Data: map[string][]byte{"tls.crt": certPEM}}
}

var _ = Describe("StatService.IsUploaderReady", func() {
	var (
		s   *StatService
		svc *corev1.Service
	)

	BeforeEach(func() {
		s = NewStatService(log.NewNop())
		svc = &corev1.Service{}
	})

	It("returns false without probing when the pod is nil", func() {
		ready, err := s.IsUploaderReady(nil, svc, ingWithURL("https://127.0.0.1:1/upload"), nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(ready).To(BeFalse())
	})

	It("returns false without probing when the pod is not ready", func() {
		ready, err := s.IsUploaderReady(readyPod(false), svc, ingWithURL("https://127.0.0.1:1/upload"), nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(ready).To(BeFalse())
	})

	It("returns true when the endpoint serves a matching, trusted cert and 200", func() {
		cert, certPEM := genCert("127.0.0.1")
		srv := tlsServer(cert, http.StatusOK)
		defer srv.Close()

		ready, err := s.IsUploaderReady(readyPod(true), svc, ingWithURL(srv.URL+"/upload"), tlsSecret(certPEM))
		Expect(err).NotTo(HaveOccurred())
		Expect(ready).To(BeTrue())
	})

	It("returns false (no error) when the endpoint replies non-200", func() {
		cert, certPEM := genCert("127.0.0.1")
		srv := tlsServer(cert, http.StatusInternalServerError)
		defer srv.Close()

		ready, err := s.IsUploaderReady(readyPod(true), svc, ingWithURL(srv.URL+"/upload"), tlsSecret(certPEM))
		Expect(err).NotTo(HaveOccurred())
		Expect(ready).To(BeFalse())
	})

	// The production bug: ingress-nginx served a certificate valid for a different
	// host ("ingress.local") than the upload host, so the probe fails TLS
	// verification. The probe must surface the error rather than report ready.
	It("returns an error when the served cert is valid for a different host", func() {
		cert, certPEM := genCert("ingress.local")
		srv := tlsServer(cert, http.StatusOK)
		defer srv.Close()

		// srv.URL is https://127.0.0.1:<port>, which the "ingress.local" cert does
		// not cover, even though we trust the cert itself via the secret.
		ready, err := s.IsUploaderReady(readyPod(true), svc, ingWithURL(srv.URL+"/upload"), tlsSecret(certPEM))
		Expect(err).To(HaveOccurred())
		Expect(ready).To(BeFalse())
	})

	It("falls back to the upload-path annotation when no upload URL is set", func() {
		ing := &netv1.Ingress{
			ObjectMeta: metav1.ObjectMeta{Annotations: map[string]string{annotations.AnnUploadPath: "/upload/token"}},
		}
		ready, err := s.IsUploaderReady(readyPod(true), svc, ing, nil)
		Expect(err).NotTo(HaveOccurred())
		Expect(ready).To(BeTrue())
	})
})
