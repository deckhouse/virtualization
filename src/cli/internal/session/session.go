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

// Package session warns a user before they disconnect somebody from the serial console or the VNC
// of a virtual machine. Both streams are exclusive: connecting takes the stream over. The platform
// answers who holds it, and this is where that answer is turned into a decision.
package session

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"
	"unicode"

	"golang.org/x/term"
	authnv1 "k8s.io/api/authentication/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	virtualizationv1alpha2 "github.com/deckhouse/virtualization/api/client/generated/clientset/versioned/typed/core/v1alpha2"
	subv1alpha2 "github.com/deckhouse/virtualization/api/subresources/v1alpha2"
	"github.com/deckhouse/virtualization/src/cli/internal/clientconfig"
)

// waitPollInterval is how often the session is asked about again while waiting for it to be freed.
var waitPollInterval = 2 * time.Second

// clientNameLimit is how many characters of an unfamiliar client's own name are spelled out.
// A long user agent still says who it is once cut short.
const clientNameLimit = 60

var spinnerChars = []rune{'⠋', '⠙', '⠹', '⠸', '⠼', '⠴', '⠦', '⠧', '⠇', '⠏'}

// Decision is what to do about a session somebody else is holding.
type Decision int

const (
	// Connect proceeds with the connection, disconnecting the holder.
	Connect Decision = iota
	// Abort leaves the holder alone and gives up on connecting.
	Abort
)

// Prober is the part of the client this package needs: the very calls it would connect with, asked
// to report who holds the stream instead.
type Prober interface {
	SerialConsole(ctx context.Context, name string, options *virtualizationv1alpha2.SerialConsoleOptions) (virtualizationv1alpha2.StreamInterface, *subv1alpha2.VirtualMachineSession, error)
	VNC(ctx context.Context, name string, options *virtualizationv1alpha2.VNCOptions) (virtualizationv1alpha2.StreamInterface, *subv1alpha2.VirtualMachineSession, error)
}

// probe asks the endpoint of the stream itself who holds it.
func probe(ctx context.Context, vms Prober, name string, kind subv1alpha2.SessionKind) (*subv1alpha2.VirtualMachineSession, error) {
	switch kind {
	case subv1alpha2.ConsoleSession:
		_, current, err := vms.SerialConsole(ctx, name, &virtualizationv1alpha2.SerialConsoleOptions{Probe: true})
		return current, err
	case subv1alpha2.VNCSession:
		_, current, err := vms.VNC(ctx, name, &virtualizationv1alpha2.VNCOptions{Probe: true})
		return current, err
	default:
		return nil, fmt.Errorf("unknown session kind %q", kind)
	}
}

// Streams are the terminal the question is asked through, so tests can drive it.
type Streams struct {
	In       io.Reader
	Out      io.Writer
	IsATTY   bool
	SelfName func(ctx context.Context) (string, error)
}

// printf writes the question out. Failing to write to a terminal is not something this can act
// upon, and it must not stand in the way of connecting.
func (s Streams) printf(format string, a ...any) {
	_, _ = fmt.Fprintf(s.Out, format, a...)
}

// DefaultStreams asks through the real terminal.
func DefaultStreams(restConfig *rest.Config) Streams {
	return Streams{
		In:     os.Stdin,
		Out:    os.Stderr,
		IsATTY: isTerminal(os.Stdin),
		SelfName: func(ctx context.Context) (string, error) {
			return selfName(ctx, restConfig)
		},
	}
}

// AskBeforeConnecting is the check as a command needs it: it prepares the terminal to ask through
// and returns the decision. The client config is only needed to recognise the very user who asks,
// so a command that cannot get it still gets an answer.
func AskBeforeConnecting(
	ctx context.Context,
	vms Prober,
	name string,
	kind subv1alpha2.SessionKind,
	force bool,
) Decision {
	restConfig, err := clientconfig.GetRESTConfig(ctx)
	if err != nil {
		restConfig = nil
	}
	return Check(ctx, vms, name, kind, force, DefaultStreams(restConfig))
}

// Check asks who holds the stream and decides whether to connect.
//
// It never stands between a user and a virtual machine: if the platform cannot answer — an older
// version that does not know the question, a denied or failed request — the connection proceeds as
// it always did. The same when the stream is free, or held by the very user who is asking.
func Check(
	ctx context.Context,
	vms Prober,
	name string,
	kind subv1alpha2.SessionKind,
	force bool,
	s Streams,
) Decision {
	if force {
		return Connect
	}

	current, err := probe(ctx, vms, name, kind)
	if err != nil || current == nil || current.Holder == "" {
		return Connect
	}

	// Reconnecting to a stream this very user holds is their own business.
	if s.SelfName != nil {
		if self, err := s.SelfName(ctx); err == nil && self == current.Holder {
			return Connect
		}
	}

	// Nobody is at the terminal to answer, so behave exactly as before and leave a trace of who
	// was disconnected: a script that used to work keeps working.
	if !s.IsATTY {
		// Plain newlines: the question is asked before the terminal is put into raw mode, and this
		// branch writes into the error output of a script, where a stray carriage return shows up.
		s.printf("Warning: disconnecting %s from the %s of %s, connected %s.\n",
			Holder(current.Holder), kind.Description(), name, when(current.StartTime))
		return Connect
	}

	return ask(ctx, vms, name, kind, current, s)
}

func ask(
	ctx context.Context,
	vms Prober,
	name string,
	kind subv1alpha2.SessionKind,
	current *subv1alpha2.VirtualMachineSession,
	s Streams,
) Decision {
	s.printf("\nThe %s of %s is in use:\n", kind.Description(), name)
	s.printf("  user       %s\n", Holder(current.Holder))
	s.printf("  connected  %s%s\n\n", when(current.StartTime), from(current.Client))

	reader := bufio.NewReader(s.In)
	for {
		s.printf("Connect and disconnect them? [y] yes  [N] no  [w] wait until free: ")
		answer, err := reader.ReadString('\n')
		if err != nil && answer == "" {
			// The answer cannot be read at all: leave the holder alone rather than guess.
			s.printf("\n")
			return Abort
		}

		switch strings.ToLower(strings.TrimSpace(answer)) {
		case "y", "yes":
			return Connect
		case "", "n", "no":
			return Abort
		case "w", "wait":
			return waitUntilFree(ctx, vms, name, kind, s)
		}
	}
}

// waitUntilFree holds off until the stream is released, so that a user who does not want to
// interrupt anybody does not have to keep running the command by hand.
func waitUntilFree(
	ctx context.Context,
	vms Prober,
	name string,
	kind subv1alpha2.SessionKind,
	s Streams,
) Decision {
	spinner := 0
	for {
		s.printf("\r\x1b[K%c Waiting for the %s of %s to be released. Press Ctrl+C to exit.",
			spinnerChars[spinner], kind.Description(), name)
		spinner = (spinner + 1) % len(spinnerChars)

		select {
		case <-ctx.Done():
			s.printf("\r\x1b[K")
			return Abort
		case <-time.After(waitPollInterval):
		}

		current, err := probe(ctx, vms, name, kind)
		if err != nil || current == nil || current.Holder == "" {
			s.printf("\r\x1b[K")
			return Connect
		}
	}
}

// Holder shortens the identity of a session holder to what a human needs to find that person.
// A service account name is unreadable in full and its long form carries nothing extra.
func Holder(holder string) string {
	const saPrefix = "system:serviceaccount:"
	if rest, ok := strings.CutPrefix(holder, saPrefix); ok {
		if ns, sa, found := strings.Cut(rest, ":"); found {
			return fmt.Sprintf("serviceaccount %s/%s", ns, sa)
		}
	}
	return holder
}

// when tells how long ago the session started, in both relative and absolute form: the first to
// judge by, the second to compare with anything else.
func when(startTime *metav1.Time) string {
	if startTime == nil {
		return "at an unknown time"
	}
	local := startTime.Local()
	return fmt.Sprintf("%s ago (%s)", roundedSince(local), local.Format("15:04"))
}

func roundedSince(t time.Time) string {
	d := time.Since(t)
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%d seconds", int(d.Seconds()))
	case d < time.Hour:
		return fmt.Sprintf("%d minutes", int(d.Minutes()))
	default:
		return fmt.Sprintf("%.1f hours", d.Hours())
	}
}

// from names the client of the holder, which is often what tells one how to go and reach them.
// A user agent is what the platform records, and it is not what a person needs to read: a familiar
// client is named in words. An unfamiliar one has nothing else to it, so the user agent itself is
// spelled out — that is all there is to go by.
func from(client string) string {
	if client == "" {
		return ""
	}
	agent, _, _ := strings.Cut(client, " ")
	name, _, _ := strings.Cut(agent, "/")
	switch name {
	case "d8":
		return ", from the d8 v command line"
	// The web console introduces itself as "console" when it opens the stream; "d8" above is the
	// client-go default, the name of this binary. Any other client falls through and is spelled
	// out as it named itself, so nothing is lost until this list catches up.
	case "console":
		return ", from the web console"
	default:
		if spelled := clientName(client); spelled != "" {
			return fmt.Sprintf(", from another client (%s)", spelled)
		}
		return ", from another client"
	}
}

// clientName prepares the name of an unfamiliar client to be printed. That name comes from the
// client itself and can hold anything at all — terminal escapes, any length — while this goes
// straight into somebody's terminal.
func clientName(client string) string {
	printable := []rune(strings.Map(func(r rune) rune {
		if unicode.IsPrint(r) {
			return r
		}
		return -1
	}, client))
	if len(printable) > clientNameLimit {
		return string(printable[:clientNameLimit]) + "…"
	}
	return string(printable)
}

// selfName is asked only when a session turns out to be held, so it costs nothing on the common
// path of a free console.
func selfName(ctx context.Context, restConfig *rest.Config) (string, error) {
	if restConfig == nil {
		return "", errors.New("no client config to ask who we are")
	}
	cli, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return "", err
	}
	review, err := cli.AuthenticationV1().SelfSubjectReviews().Create(ctx, &authnv1.SelfSubjectReview{}, metav1.CreateOptions{})
	if err != nil {
		return "", err
	}
	return review.Status.UserInfo.Username, nil
}

// isTerminal answers whether there is somebody at the other end to answer the question. Asked of
// the file descriptor itself, not of its mode: `/dev/null` and the like are character devices too,
// and a script run with `< /dev/null` would be taken for a person, asked a question nobody answers
// and left without a connection.
func isTerminal(f *os.File) bool {
	return term.IsTerminal(int(f.Fd()))
}
