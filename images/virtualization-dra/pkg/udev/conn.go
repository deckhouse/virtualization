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
	"log/slog"
	"os"
	"runtime"
	"syscall"

	"golang.org/x/sys/unix"
)

// Mode determines the event source
type Mode int

const (
	// KernelEvent - raw kernel events (faster, less info)
	KernelEvent Mode = 1
	// UdevEvent - events processed by udevd (richer, with more attributes like vendor info, serial numbers)
	UdevEvent Mode = 2
)

// HostNetNS is a path to the host network namespace.
// Use this to receive udev events when running in a container without host networking.
const HostNetNS = "/proc/1/ns/net"

// Conn represents a netlink connection for uevents
type Conn struct {
	fd    int
	addr  syscall.SockaddrNetlink
	netNS string // optional: path to network namespace (e.g., /proc/1/ns/net)
}

// ConnOption is a functional option for Conn
type ConnOption func(*Conn)

// WithNetNS sets the network namespace path for the connection.
// The socket will be created in the specified network namespace.
// This is useful when running in a container without host networking -
// use HostNetNS ("/proc/1/ns/net") to receive udev events from the host.
func WithNetNS(path string) ConnOption {
	return func(c *Conn) {
		c.netNS = path
	}
}

// NewConn creates a new udev connection
func NewConn(opts ...ConnOption) *Conn {
	c := &Conn{}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// Connect establishes a connection to the netlink socket
func (c *Conn) Connect(mode Mode) error {
	if c.netNS != "" {
		return c.connectInNetNS(mode)
	}
	return c.connect(mode)
}

func (c *Conn) connect(mode Mode) error {
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

// connectInNetNS creates the netlink socket in the specified network namespace.
// This allows receiving udev events from the host when running in a container.
//
// IMPORTANT: If the function cannot restore the original network namespace,
// it will panic because the OS thread would be left in an undefined state.
func (c *Conn) connectInNetNS(mode Mode) error {
	// Lock the OS thread to ensure namespace operations affect only this goroutine.
	// This is critical because Go scheduler can migrate goroutines between OS threads,
	// and namespace is a per-thread property.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	// Save current network namespace
	currentNS, err := unix.Open("/proc/self/ns/net", unix.O_RDONLY|unix.O_CLOEXEC, 0)
	if err != nil {
		return fmt.Errorf("failed to open current netns: %w", err)
	}
	defer func(fd int) {
		if err = unix.Close(fd); err != nil {
			slog.Error("failed to close current netns", slog.String("error", err.Error()))
		}
	}(currentNS)

	// Open target network namespace
	targetNS, err := unix.Open(c.netNS, unix.O_RDONLY|unix.O_CLOEXEC, 0)
	if err != nil {
		return fmt.Errorf("failed to open target netns %s: %w", c.netNS, err)
	}
	defer func(fd int) {
		if err = unix.Close(fd); err != nil {
			slog.Error("failed to close target netns", slog.String("error", err.Error()))
		}
	}(targetNS)

	// Switch to target network namespace
	if err := unix.Setns(targetNS, unix.CLONE_NEWNET); err != nil {
		return fmt.Errorf("failed to switch to target netns: %w", err)
	}

	defer func() {
		if err := unix.Setns(currentNS, unix.CLONE_NEWNET); err != nil {
			panic(fmt.Sprintf("FATAL: failed to restore original netns: %v", err))
		}
	}()

	// Create socket in target namespace.
	// The socket will remain bound to the target namespace even after
	// we switch back to the original namespace.
	return c.connect(mode)
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
