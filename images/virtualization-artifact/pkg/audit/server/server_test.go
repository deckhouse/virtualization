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

package server

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"io"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// recorder is a thread-safe handler that records every audit line it receives.
type recorder struct {
	mu    sync.Mutex
	lines [][]byte
}

func (r *recorder) handle(b []byte) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	// scanner.Bytes() reuses its buffer, so copy before keeping the slice.
	r.lines = append(r.lines, append([]byte(nil), b...))
	return nil
}

func (r *recorder) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.lines)
}

func (r *recorder) get() [][]byte {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([][]byte, len(r.lines))
	copy(out, r.lines)
	return out
}

// freeAddr reserves a free loopback port and returns its address. There is a
// small race before the server rebinds it, acceptable for a unit test.
func freeAddr() string {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	Expect(err).NotTo(HaveOccurred())
	addr := l.Addr().String()
	Expect(l.Close()).To(Succeed())
	return addr
}

// issue creates a certificate. With parent == nil it is a self-signed CA;
// otherwise it is a leaf signed by parent (server or client depending on flags).
func issue(cn string, parent *x509.Certificate, parentKey *ecdsa.PrivateKey, isCA, server bool) (*x509.Certificate, *ecdsa.PrivateKey, []byte, []byte) {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	Expect(err).NotTo(HaveOccurred())

	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 62))
	Expect(err).NotTo(HaveOccurred())

	tmpl := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: cn},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(time.Hour),
		BasicConstraintsValid: true,
	}
	switch {
	case isCA:
		tmpl.IsCA = true
		tmpl.KeyUsage = x509.KeyUsageCertSign | x509.KeyUsageDigitalSignature
	case server:
		tmpl.KeyUsage = x509.KeyUsageDigitalSignature
		tmpl.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}
		tmpl.IPAddresses = []net.IP{net.ParseIP("127.0.0.1")}
	default:
		tmpl.KeyUsage = x509.KeyUsageDigitalSignature
		tmpl.ExtKeyUsage = []x509.ExtKeyUsage{x509.ExtKeyUsageClientAuth}
	}

	signer, signerKey := tmpl, key
	if parent != nil {
		signer, signerKey = parent, parentKey
	}

	der, err := x509.CreateCertificate(rand.Reader, tmpl, signer, &key.PublicKey, signerKey)
	Expect(err).NotTo(HaveOccurred())
	cert, err := x509.ParseCertificate(der)
	Expect(err).NotTo(HaveOccurred())

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	Expect(err).NotTo(HaveOccurred())
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})

	return cert, key, certPEM, keyPEM
}

func writeFile(dir, name string, data []byte) string {
	p := filepath.Join(dir, name)
	Expect(os.WriteFile(p, data, 0o600)).To(Succeed())
	return p
}

// startServer runs the server in a goroutine on a free address and returns the
// address, a cancel func, and a channel that yields Run's return value.
func startServer(handler func([]byte) error, opts ...Option) (string, context.CancelFunc, <-chan error) {
	addr := freeAddr()
	srv, err := NewServer(addr, handler)
	Expect(err).NotTo(HaveOccurred())

	ctx, cancel := context.WithCancel(context.Background())
	runErr := make(chan error, 1)
	go func() { runErr <- srv.Run(ctx, opts...) }()
	return addr, cancel, runErr
}

var _ = Describe("Audit server", func() {
	var rec *recorder

	BeforeEach(func() {
		rec = &recorder{}
	})

	Context("plain TCP", func() {
		var (
			addr   string
			cancel context.CancelFunc
			runErr <-chan error
		)

		BeforeEach(func() {
			addr, cancel, runErr = startServer(rec.handle)
			// Wait until the listener accepts connections.
			Eventually(func() error {
				c, err := net.Dial("tcp", addr)
				if err == nil {
					_ = c.Close()
				}
				return err
			}, "2s", "20ms").Should(Succeed())
		})

		AfterEach(func() {
			cancel()
			Eventually(runErr, "2s").Should(Receive(BeNil()))
		})

		It("delivers newline-delimited lines to the handler in order", func() {
			conn, err := net.Dial("tcp", addr)
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = conn.Close() }()

			_, err = conn.Write([]byte("first\nsecond\nthird\n"))
			Expect(err).NotTo(HaveOccurred())

			Eventually(rec.count, "2s").Should(Equal(3))
			lines := rec.get()
			Expect(string(lines[0])).To(Equal("first"))
			Expect(string(lines[1])).To(Equal("second"))
			Expect(string(lines[2])).To(Equal("third"))
		})

		It("delivers a line larger than the 64 KiB scanner default intact", func() {
			big := strings.Repeat("a", 256*1024)

			conn, err := net.Dial("tcp", addr)
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = conn.Close() }()

			_, err = conn.Write([]byte(big + "\n"))
			Expect(err).NotTo(HaveOccurred())

			Eventually(rec.count, "2s").Should(Equal(1))
			Expect(rec.get()[0]).To(HaveLen(len(big)))
		})

		It("does not deliver a line exceeding maxAuditEventSize", func() {
			conn, err := net.Dial("tcp", addr)
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = conn.Close() }()

			oversized := strings.Repeat("a", maxAuditEventSize+1)
			_, _ = conn.Write([]byte(oversized + "\n"))

			Consistently(rec.count, "500ms").Should(Equal(0))
		})
	})

	Context("graceful shutdown", func() {
		It("returns promptly and closes active connections cleanly", func() {
			addr, cancel, runErr := startServer(rec.handle)
			Eventually(func() error {
				c, err := net.Dial("tcp", addr)
				if err == nil {
					_ = c.Close()
				}
				return err
			}, "2s", "20ms").Should(Succeed())

			conn, err := net.Dial("tcp", addr)
			Expect(err).NotTo(HaveOccurred())
			defer func() { _ = conn.Close() }()

			_, err = conn.Write([]byte("ping\n"))
			Expect(err).NotTo(HaveOccurred())
			Eventually(rec.count, "2s").Should(Equal(1))

			cancel()

			// Run must return well within gracefulShutdownTimeout (5s).
			Eventually(runErr, "3s").Should(Receive(BeNil()))

			// The active connection is closed by the server, so the client sees EOF.
			Expect(conn.SetReadDeadline(time.Now().Add(2 * time.Second))).To(Succeed())
			_, err = conn.Read(make([]byte, 1))
			Expect(err).To(MatchError(io.EOF))
		})
	})

	Context("waitWithTimeout", func() {
		It("returns immediately once the WaitGroup drains", func() {
			var wg sync.WaitGroup
			start := time.Now()
			waitWithTimeout(&wg, time.Second)
			Expect(time.Since(start)).To(BeNumerically("<", 500*time.Millisecond))
		})

		It("returns after the timeout when the WaitGroup never drains", func() {
			var wg sync.WaitGroup
			wg.Add(1)
			defer wg.Done()

			start := time.Now()
			waitWithTimeout(&wg, 100*time.Millisecond)
			Expect(time.Since(start)).To(BeNumerically(">=", 100*time.Millisecond))
		})
	})

	Context("mTLS", func() {
		var (
			addr      string
			cancel    context.CancelFunc
			runErr    <-chan error
			caPEM     []byte
			clientPEM []byte
			clientKey []byte
			ca        *x509.Certificate
			caKey     *ecdsa.PrivateKey
		)

		BeforeEach(func() {
			var caCertPEM []byte
			ca, caKey, caCertPEM, _ = issue("test-ca", nil, nil, true, false)
			caPEM = caCertPEM

			_, _, serverPEM, serverKey := issue("audit-server", ca, caKey, false, true)
			_, _, clientPEM, clientKey = issue("audit-client", ca, caKey, false, false)

			dir := GinkgoT().TempDir()
			caFile := writeFile(dir, "ca.crt", caPEM)
			certFile := writeFile(dir, "tls.crt", serverPEM)
			keyFile := writeFile(dir, "tls.key", serverKey)

			addr, cancel, runErr = startServer(rec.handle, WithTLS(caFile, certFile, keyFile))
		})

		AfterEach(func() {
			cancel()
			Eventually(runErr, "3s").Should(Receive(BeNil()))
		})

		caPool := func() *x509.CertPool {
			pool := x509.NewCertPool()
			Expect(pool.AppendCertsFromPEM(caPEM)).To(BeTrue())
			return pool
		}

		dialTLS := func(cfg *tls.Config) (*tls.Conn, error) {
			cfg.ServerName = "127.0.0.1"
			d := &net.Dialer{Timeout: 2 * time.Second}
			return tls.DialWithDialer(d, "tcp", addr, cfg)
		}

		// mTLSRejected reports whether the server refuses the client. Under TLS 1.3
		// a client-cert rejection is delivered as a post-handshake alert, so dialing
		// may succeed and the error only surfaces on the first read.
		mTLSRejected := func(cfg *tls.Config) bool {
			conn, err := dialTLS(cfg)
			if err != nil {
				return true
			}
			defer func() { _ = conn.Close() }()
			Expect(conn.SetDeadline(time.Now().Add(2 * time.Second))).To(Succeed())
			_, err = conn.Write([]byte("audit-event\n"))
			if err == nil {
				_, err = conn.Read(make([]byte, 1))
			}
			return err != nil && !errors.Is(err, io.EOF)
		}

		It("accepts a client with a cert signed by the CA and delivers its lines", func() {
			pair, err := tls.X509KeyPair(clientPEM, clientKey)
			Expect(err).NotTo(HaveOccurred())

			var conn *tls.Conn
			Eventually(func() error {
				conn, err = dialTLS(&tls.Config{
					Certificates: []tls.Certificate{pair},
					RootCAs:      caPool(),
				})
				return err
			}, "2s", "20ms").Should(Succeed())
			defer func() { _ = conn.Close() }()

			_, err = conn.Write([]byte("audit-event\n"))
			Expect(err).NotTo(HaveOccurred())

			Eventually(rec.count, "2s").Should(Equal(1))
			Expect(string(rec.get()[0])).To(Equal("audit-event"))
		})

		It("rejects a client presenting no certificate", func() {
			Eventually(func() bool {
				return mTLSRejected(&tls.Config{RootCAs: caPool()})
			}, "2s", "50ms").Should(BeTrue())
			Expect(rec.count()).To(Equal(0))
		})

		It("rejects a client whose cert is signed by a different CA", func() {
			otherCA, otherKey, _, _ := issue("other-ca", nil, nil, true, false)
			_, _, otherClientPEM, otherClientKey := issue("intruder", otherCA, otherKey, false, false)
			pair, err := tls.X509KeyPair(otherClientPEM, otherClientKey)
			Expect(err).NotTo(HaveOccurred())

			Eventually(func() bool {
				return mTLSRejected(&tls.Config{
					Certificates: []tls.Certificate{pair},
					RootCAs:      caPool(),
				})
			}, "2s", "50ms").Should(BeTrue())
			Expect(rec.count()).To(Equal(0))
		})
	})
})
