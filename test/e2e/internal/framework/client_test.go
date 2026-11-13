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

package framework

import (
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

var errDial = &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("i/o timeout")}

// failing returns a transport that fails the first n requests with err and records each body.
func failing(n int, err error, bodies *[]string) http.RoundTripper {
	return roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.Body != nil {
			b, _ := io.ReadAll(req.Body)
			_ = req.Body.Close()

			*bodies = append(*bodies, string(b))
		} else {
			*bodies = append(*bodies, "")
		}

		if len(*bodies) <= n {
			return nil, err
		}

		return &http.Response{StatusCode: http.StatusOK, Body: http.NoBody}, nil
	})
}

// send runs one POST through rt and returns the status code, 0 on error.
func send(rt http.RoundTripper) (int, error) {
	GinkgoHelper()

	req, err := http.NewRequest(http.MethodPost, "https://api.example/apis/x", strings.NewReader("payload"))
	Expect(err).NotTo(HaveOccurred())

	resp, err := rt.RoundTrip(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	return resp.StatusCode, nil
}

var _ = Describe("retryOnDialError", func() {
	var bodies []string

	BeforeEach(func() {
		bodies = nil
	})

	It("repeats a request that failed to connect, with the same body", func() {
		rt := retryOnDialError{next: failing(2, errDial, &bodies), window: time.Second, interval: time.Millisecond}

		Expect(send(rt)).To(Equal(http.StatusOK))
		Expect(bodies).To(Equal([]string{"payload", "payload", "payload"}))
	})

	It("does not repeat a request that reached the server", func() {
		// A repeat can create an object twice.
		sent := &net.OpError{Op: "read", Net: "tcp", Err: errors.New("connection reset by peer")}
		rt := retryOnDialError{next: failing(1, sent, &bodies), window: time.Second, interval: time.Millisecond}

		_, err := send(rt)
		Expect(err).To(MatchError(sent))
		Expect(bodies).To(HaveLen(1))
	})

	It("stops repeating after the window", func() {
		rt := retryOnDialError{next: failing(1000, errDial, &bodies), window: 50 * time.Millisecond, interval: 10 * time.Millisecond}

		_, err := send(rt)
		Expect(isDialError(err)).To(BeTrue(), "want the dial error, got %v", err)
		Expect(len(bodies)).To(BeNumerically(">=", 2))
		Expect(len(bodies)).To(BeNumerically("<=", 7))
	})
})
