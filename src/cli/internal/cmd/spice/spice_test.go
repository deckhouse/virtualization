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

package spice

import (
	"context"
	"io"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/spf13/cobra"

	"github.com/deckhouse/virtualization/api/client/kubeclient"
)

// The whole point of this command over `d8 v vnc` is that one SPICE session is
// several connections: the listener must keep accepting and open a stream per
// connection instead of serving a single one. A regression here would silently
// degrade SPICE to "only the first channel works".
func TestAcceptLoopServesEveryConnection(t *testing.T) {
	const channels = 4

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	var mu sync.Mutex
	served := 0
	done := make(chan struct{})

	orig := pipeChannelFunc
	defer func() { pipeChannelFunc = orig }()
	pipeChannelFunc = func(_ context.Context, _ kubeclient.Client, _, _ string, conn net.Conn) error {
		mu.Lock()
		served++
		reached := served
		mu.Unlock()
		if reached == channels {
			close(done)
		}
		// hold the connection open, mimicking a live channel
		<-done
		return nil
	}

	cmd := &cobra.Command{}
	cmd.SetOut(io.Discard)
	go acceptLoop(context.Background(), ln, nil, "default", "myvm", cmd, make(chan error, 1))

	conns := make([]net.Conn, 0, channels)
	for i := 0; i < channels; i++ {
		c, err := net.Dial("tcp", ln.Addr().String())
		if err != nil {
			t.Fatalf("dial %d: %v", i, err)
		}
		conns = append(conns, c)
	}
	defer func() {
		for _, c := range conns {
			c.Close()
		}
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		mu.Lock()
		got := served
		mu.Unlock()
		t.Fatalf("expected %d concurrent channels, served %d", channels, got)
	}
}
