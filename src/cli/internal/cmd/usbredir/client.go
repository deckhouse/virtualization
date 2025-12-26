/*
Copyright 2018 The KubeVirt Authors
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

Initially copied from https://github.com/kubevirt/kubevirt/blob/v1.6.2/pkg/virtctl/usbredir/client.go
*/

package usbredir

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"time"

	"github.com/deckhouse/virtualization/api/client/generated/clientset/versioned/typed/core/v1alpha2"
)

type Client struct {
	inputReader  *io.PipeReader
	inputWriter  *io.PipeWriter
	outputReader *io.PipeReader
	outputWriter *io.PipeWriter

	usbRedirector Redirector
	remoteStream  v1alpha2.StreamInterface

	// channels
	done   chan struct{}
	stream chan error
	remote chan error
}

func NewClient(remoteStream v1alpha2.StreamInterface, redirector Redirector) *Client {
	inReader, inWriter := io.Pipe()
	outReader, outWriter := io.Pipe()
	return &Client{
		inputReader:  inReader,
		inputWriter:  inWriter,
		outputReader: outReader,
		outputWriter: outWriter,

		usbRedirector: redirector,
		remoteStream:  remoteStream,
	}
}

func (c *Client) Redirect(ctx context.Context) error {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return fmt.Errorf("can't listen on random port: %w", err)
	}
	defer ln.Close()

	c.startRemoteStream(ctx)
	c.startProxyUSBRedir(ctx, ln)

	address := ln.Addr().String()

	usbRedirectorCh := make(chan error)
	go func() {
		defer close(c.done)
		usbRedirectorCh <- c.usbRedirector.Redirect(ctx, address)
	}()

	select {
	case err = <-c.stream:
	case err = <-usbRedirectorCh:
	case err = <-c.remote:
	case <-ctx.Done():
	}
	return err
}

func (c *Client) startRemoteStream(ctx context.Context) {
	c.stream = make(chan error)

	go func() {
		defer c.outputWriter.Close()
		select {
		case c.stream <- c.remoteStream.Stream(
			v1alpha2.StreamOptions{
				In:  c.inputReader,
				Out: c.outputWriter,
			},
		):
		case <-ctx.Done():
		}
	}()
}

func (c *Client) startProxyUSBRedir(ctx context.Context, listener net.Listener) {
	c.done = make(chan struct{}, 1)
	c.remote = make(chan error)
	go func() {
		defer c.inputWriter.Close()
		start := time.Now()

		usbredirConn, err := listener.Accept()
		if err != nil {
			slog.Info("Failed to accept connection", slog.Any("err", err))
			c.remote <- err
			return
		}
		defer usbredirConn.Close()

		slog.Info("Connected to usbredir at", slog.Any("time", time.Since(start)))

		stream := make(chan error)
		// write to local usbredir from pipeOutReader
		go func() {
			_, err := io.Copy(usbredirConn, c.outputReader)
			stream <- err
		}()

		// read from local usbredir towards pipeInWriter
		go func() {
			_, err := io.Copy(c.inputWriter, usbredirConn)
			stream <- err
		}()

		select {
		case <-c.done: // Wait for local usbredir to complete
		case err = <-stream: // Wait for remote connection to close
			if err == nil {
				// Remote connection closed, report this as error
				err = fmt.Errorf("remote connection has closed")
			}
			// Wait for local usbredir to complete
			c.remote <- err
		case <-ctx.Done():
		}
	}()
}
