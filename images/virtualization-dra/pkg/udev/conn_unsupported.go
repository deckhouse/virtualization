//go:build !linux

/*
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
*/

package udev

import "fmt"

// Mode determines the event source.
type Mode int

const (
	// KernelEvent - raw kernel events (faster, less info)
	KernelEvent Mode = 1
	// UdevEvent - events processed by udevd (richer, with more attributes like vendor info, serial numbers)
	UdevEvent Mode = 2
)

// HostNetNS is a path to the host network namespace.
const HostNetNS = "/proc/1/ns/net"

// Conn represents a netlink connection for uevents.
type Conn struct {
	fd    int
	netNS string
}

// ConnOption is a functional option for Conn.
type ConnOption func(*Conn)

// WithNetNS sets the network namespace path for the connection.
func WithNetNS(path string) ConnOption {
	return func(c *Conn) {
		c.netNS = path
	}
}

// NewConn creates a new udev connection.
func NewConn(opts ...ConnOption) *Conn {
	c := &Conn{}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// Connect is unsupported on non-Linux platforms.
func (c *Conn) Connect(_ Mode) error {
	return fmt.Errorf("udev netlink is supported only on linux")
}

// Close closes the connection.
func (c *Conn) Close() error {
	return nil
}

// ReadMsg is unsupported on non-Linux platforms.
func (c *Conn) ReadMsg() ([]byte, error) {
	return nil, fmt.Errorf("udev netlink is supported only on linux")
}

// ReadUEvent is unsupported on non-Linux platforms.
func (c *Conn) ReadUEvent() (*UEvent, error) {
	return nil, fmt.Errorf("udev netlink is supported only on linux")
}

// Fd returns the file descriptor of the connection.
func (c *Conn) Fd() int {
	return c.fd
}
