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

package session

import (
	"bytes"
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/utils/ptr"

	virtualizationv1alpha2 "github.com/deckhouse/virtualization/api/client/generated/clientset/versioned/typed/core/v1alpha2"
	subv1alpha2 "github.com/deckhouse/virtualization/api/subresources/v1alpha2"
)

func TestSession(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Session Suite")
}

// fakeProber answers about a session the way a platform would, one answer per call. Both streams
// answer the same way: what is asked about is chosen by the call, not by the answer.
type fakeProber struct {
	answers []*subv1alpha2.VirtualMachineSession
	err     error
	calls   int
}

func (f *fakeProber) SerialConsole(context.Context, string, *virtualizationv1alpha2.SerialConsoleOptions) (virtualizationv1alpha2.StreamInterface, *subv1alpha2.VirtualMachineSession, error) {
	current, err := f.answer()
	return nil, current, err
}

func (f *fakeProber) VNC(context.Context, string, *virtualizationv1alpha2.VNCOptions) (virtualizationv1alpha2.StreamInterface, *subv1alpha2.VirtualMachineSession, error) {
	current, err := f.answer()
	return nil, current, err
}

func (f *fakeProber) answer() (*subv1alpha2.VirtualMachineSession, error) {
	f.calls++
	if f.err != nil {
		return nil, f.err
	}
	if f.calls > len(f.answers) {
		return f.answers[len(f.answers)-1], nil
	}
	return f.answers[f.calls-1], nil
}

var _ = Describe("Check", func() {
	var (
		out  *bytes.Buffer
		free *subv1alpha2.VirtualMachineSession
	)

	held := func(holder string) *subv1alpha2.VirtualMachineSession {
		return &subv1alpha2.VirtualMachineSession{
			Holder:    holder,
			StartTime: ptr.To(metav1.NewTime(time.Now().Add(-12 * time.Minute))),
			Client:    "d8/v1.33.8 (linux/amd64)",
		}
	}

	// streams drives the question as a terminal would: answer typed in, self reported as "me".
	streams := func(answer string, atty bool, self string) Streams {
		return Streams{
			In:     strings.NewReader(answer),
			Out:    out,
			IsATTY: atty,
			SelfName: func(context.Context) (string, error) {
				if self == "" {
					return "", errors.New("unknown")
				}
				return self, nil
			},
		}
	}

	check := func(prober *fakeProber, force bool, s Streams) Decision {
		return Check(context.Background(), prober, "vm", subv1alpha2.ConsoleSession, force, s)
	}

	BeforeEach(func() {
		out = &bytes.Buffer{}
		free = &subv1alpha2.VirtualMachineSession{}
		waitPollInterval = time.Millisecond
	})

	It("connects to a free console without a word", func() {
		Expect(check(&fakeProber{answers: []*subv1alpha2.VirtualMachineSession{free}}, false, streams("", true, "me"))).To(Equal(Connect))
		Expect(out.String()).To(BeEmpty())
	})

	It("names the holder and their client, and aborts on refusal", func() {
		prober := &fakeProber{answers: []*subv1alpha2.VirtualMachineSession{held("system:serviceaccount:ns:alice")}}

		Expect(check(prober, false, streams("n\n", true, "me"))).To(Equal(Abort))
		Expect(out.String()).To(SatisfyAll(
			ContainSubstring("serviceaccount ns/alice"),
			ContainSubstring("12 minutes ago"),
			ContainSubstring("from the d8 v command line"),
		))
	})

	It("names where the holder connected from, in words for a client it knows", func() {
		heldFrom := func(client string) *fakeProber {
			session := held("alice")
			session.Client = client
			return &fakeProber{answers: []*subv1alpha2.VirtualMachineSession{session}}
		}

		Expect(check(heldFrom("console"), false, streams("n\n", true, "me"))).To(Equal(Abort))
		Expect(out.String()).To(SatisfyAll(
			ContainSubstring("from the web console"),
			Not(ContainSubstring("(console)")),
		))

		out.Reset()

		// The name of a client comes with a version and a platform behind it, which must not stand
		// in the way of recognising it.
		Expect(check(heldFrom("console/v1.57.0 (linux/amd64) kubernetes/$Format"), false, streams("n\n", true, "me"))).To(Equal(Abort))
		Expect(out.String()).To(ContainSubstring("from the web console"))

		out.Reset()

		// An unfamiliar client is spelled out: "another client" alone tells one nothing.
		Expect(check(heldFrom("Go-http-client/2.0"), false, streams("n\n", true, "me"))).To(Equal(Abort))
		Expect(out.String()).To(ContainSubstring("from another client (Go-http-client/2.0)"))
	})

	It("prints the name of an unfamiliar client without letting it drive the terminal", func() {
		heldFrom := func(client string) *fakeProber {
			session := held("alice")
			session.Client = client
			return &fakeProber{answers: []*subv1alpha2.VirtualMachineSession{session}}
		}

		long := "kubectl/1.33.2 (linux/amd64) " + strings.Repeat("and on ", 10)
		Expect(check(heldFrom("\x1b[31m\r"+long), false, streams("n\n", true, "me"))).To(Equal(Abort))
		Expect(out.String()).To(SatisfyAll(
			ContainSubstring("from another client ("),
			ContainSubstring("kubectl/1.33.2 (linux/amd64)"),
			// What escapes are made of is left alone — harmless as plain text — but nothing of them
			// reaches the terminal as a command, and the name is cut short.
			Not(ContainSubstring("\x1b")),
			ContainSubstring("…)"),
			Not(ContainSubstring(strings.Repeat("and on ", 10))),
		))
	})

	It("says nothing of a client that names itself with escapes alone", func() {
		session := held("alice")
		session.Client = "\x1b[31m"
		prober := &fakeProber{answers: []*subv1alpha2.VirtualMachineSession{session}}

		Expect(check(prober, false, streams("n\n", true, "me"))).To(Equal(Abort))
		Expect(out.String()).To(SatisfyAll(
			ContainSubstring("from another client"),
			Not(ContainSubstring("()")),
		))
	})

	It("treats a bare Enter as no, because connecting is what cannot be undone", func() {
		prober := &fakeProber{answers: []*subv1alpha2.VirtualMachineSession{held("alice")}}
		Expect(check(prober, false, streams("\n", true, "me"))).To(Equal(Abort))
	})

	It("connects once confirmed", func() {
		prober := &fakeProber{answers: []*subv1alpha2.VirtualMachineSession{held("alice")}}
		Expect(check(prober, false, streams("y\n", true, "me"))).To(Equal(Connect))
	})

	It("does not question a session held by the very same user", func() {
		prober := &fakeProber{answers: []*subv1alpha2.VirtualMachineSession{held("alice")}}

		Expect(check(prober, false, streams("", true, "alice"))).To(Equal(Connect))
		Expect(out.String()).To(BeEmpty())
	})

	It("waits until the session is released and then connects", func() {
		prober := &fakeProber{answers: []*subv1alpha2.VirtualMachineSession{held("alice"), free}}

		Expect(check(prober, false, streams("w\n", true, "me"))).To(Equal(Connect))
		Expect(out.String()).To(ContainSubstring("Waiting for the serial console"))
	})

	It("keeps a script working, warning who was disconnected", func() {
		prober := &fakeProber{answers: []*subv1alpha2.VirtualMachineSession{held("system:serviceaccount:ns:alice")}}

		Expect(check(prober, false, streams("", false, "me"))).To(Equal(Connect))
		Expect(out.String()).To(ContainSubstring("Warning: disconnecting serviceaccount ns/alice"))
	})

	It("connects in silence when the platform cannot answer", func() {
		// An older platform does not know the question. It must not cost anybody their console.
		prober := &fakeProber{err: errors.New("Upgrade request required")}

		Expect(check(prober, false, streams("", true, "me"))).To(Equal(Connect))
		Expect(out.String()).To(BeEmpty())
	})

	It("does not even ask the platform when forced", func() {
		prober := &fakeProber{answers: []*subv1alpha2.VirtualMachineSession{held("alice")}}

		Expect(check(prober, true, streams("", true, "me"))).To(Equal(Connect))
		Expect(prober.calls).To(BeZero())
	})
})

var _ = Describe("Holder", func() {
	It("shortens a service account to what a human reads", func() {
		Expect(Holder("system:serviceaccount:ns:alice")).To(Equal("serviceaccount ns/alice"))
	})

	It("leaves a user name alone", func() {
		Expect(Holder("alice@flant.com")).To(Equal("alice@flant.com"))
	})
})

var _ = Describe("isTerminal", func() {
	It("does not take a character device for a person", func() {
		// A script run with `< /dev/null` is not interactive, although /dev/null is a character
		// device: asking it a question would leave the run without a connection.
		devNull, err := os.Open(os.DevNull)
		Expect(err).NotTo(HaveOccurred())
		defer devNull.Close()

		Expect(isTerminal(devNull)).To(BeFalse())
	})

	It("does not take a pipe for a person", func() {
		reader, writer, err := os.Pipe()
		Expect(err).NotTo(HaveOccurred())
		defer reader.Close()
		defer writer.Close()

		Expect(isTerminal(reader)).To(BeFalse())
	})
})
