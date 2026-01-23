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

import (
	"fmt"
	"os"
	"syscall"
)

// Mode determines the event source
type Mode int

const (
	// KernelEvent - raw kernel events (faster, less info)
	KernelEvent Mode = 1
	// UdevEvent - events processed by udevd (richer, with more attributes like vendor info, serial numbers)
	UdevEvent Mode = 2
)

// Conn represents a netlink connection for uevents
type Conn struct {
	fd   int
	addr syscall.SockaddrNetlink
}

// NewConn creates a new udev connection
func NewConn() *Conn {
	return &Conn{}
}

// Connect establishes a connection to the netlink socket
func (c *Conn) Connect(mode Mode) error {
	var err error
	c.fd, err = syscall.Socket(syscall.AF_NETLINK, syscall.SOCK_RAW, syscall.NETLINK_KOBJECT_UEVENT)
	if err != nil {
		return fmt.Errorf("failed to create netlink socket: %w", err)
	}

	c.addr = syscall.SockaddrNetlink{
		Family: syscall.AF_NETLINK,
		Groups: uint32(mode),
	}

	if err := syscall.Bind(c.fd, &c.addr); err != nil {
		_ = syscall.Close(c.fd)
		return fmt.Errorf("failed to bind netlink socket: %w", err)
	}

	return nil
}

// Close closes the netlink connection
func (c *Conn) Close() error {
	if c.fd != 0 {
		return syscall.Close(c.fd)
	}
	return nil
}

// ReadMsg reads a raw message from the netlink socket (blocking)
func (c *Conn) ReadMsg() ([]byte, error) {
	buf := make([]byte, os.Getpagesize())

	// Peek to get actual message size
	n, _, err := syscall.Recvfrom(c.fd, buf, syscall.MSG_PEEK)
	if err != nil {
		return nil, err
	}

	// If message is larger than buffer, resize
	for n >= len(buf) {
		buf = make([]byte, len(buf)+os.Getpagesize())
		n, _, err = syscall.Recvfrom(c.fd, buf, syscall.MSG_PEEK)
		if err != nil {
			return nil, err
		}
	}

	// Now read the actual message
	n, _, err = syscall.Recvfrom(c.fd, buf, 0)
	if err != nil {
		return nil, err
	}

	return buf[:n], nil
}

// ReadUEvent reads and parses a uevent from the socket (blocking)
func (c *Conn) ReadUEvent() (*UEvent, error) {
	msg, err := c.ReadMsg()
	if err != nil {
		return nil, err
	}
	return ParseUEvent(msg)
}

// Fd returns the file descriptor of the connection
func (c *Conn) Fd() int {
	return c.fd
}
