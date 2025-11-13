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

package udev

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"syscall"
)

// Monitor listens for uevents from netlink and sends matching events to a channel.
// It runs until the context is canceled or an unrecoverable error occurs.
type Monitor struct {
	conn     *Conn
	mode     Mode
	matcher  Matcher
	log      *slog.Logger
	connOpts []ConnOption
}

// MonitorOption is a functional option for Monitor
type MonitorOption func(*Monitor)

// WithMode sets the netlink mode
func WithMode(mode Mode) MonitorOption {
	return func(m *Monitor) {
		m.mode = mode
	}
}

// WithLogger sets the logger
func WithLogger(log *slog.Logger) MonitorOption {
	return func(m *Monitor) {
		m.log = log
	}
}

// WithConnOptions sets the connection options for the underlying Conn.
// Use this to configure network namespace for the netlink socket.
func WithConnOptions(opts ...ConnOption) MonitorOption {
	return func(m *Monitor) {
		m.connOpts = append(m.connOpts, opts...)
	}
}

// NewMonitor creates a new udev monitor
func NewMonitor(matcher Matcher, opts ...MonitorOption) *Monitor {
	m := &Monitor{
		mode:    KernelEvent,
		matcher: matcher,
		log:     slog.Default(),
	}

	for _, opt := range opts {
		opt(m)
	}

	return m
}

// Run starts the monitor and sends matching events to the provided channel.
// It blocks until the context is canceled or an error occurs.
// The channel is NOT closed when the monitor stops - the caller is responsible for that.
func (m *Monitor) Run(ctx context.Context, eventCh chan<- *UEvent) error {
	m.conn = NewConn(m.connOpts...)
	if err := m.conn.Connect(m.mode); err != nil {
		return err
	}
	defer func() {
		if err := m.conn.Close(); err != nil {
			m.log.Error("failed to close udev connection", slog.String("error", err.Error()))
		}
	}()

	m.log.Info("udev monitor started", slog.Int("mode", int(m.mode)))

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		uevent, err := m.conn.ReadUEvent()
		if err != nil {
			if errors.Is(err, syscall.EINTR) {
				continue
			}
			// Check if context was canceled
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}
			return fmt.Errorf("failed to read uevent: %w", err)
		}

		if m.matcher == nil || m.matcher.Match(uevent) {
			select {
			case eventCh <- uevent:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}
}

// Start starts the monitor in a goroutine and returns channels for events and errors.
// The caller must consume from both channels to prevent blocking.
func (m *Monitor) Start(ctx context.Context) (<-chan *UEvent, <-chan error) {
	eventCh := make(chan *UEvent, 100)
	errCh := make(chan error, 1)

	go func() {
		defer close(eventCh)
		defer close(errCh)

		if err := m.Run(ctx, eventCh); err != nil && !errors.Is(err, context.Canceled) {
			errCh <- err
		}
	}()

	return eventCh, errCh
}
