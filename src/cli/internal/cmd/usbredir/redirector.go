package usbredir

import (
	"context"
	"fmt"
	"net"
	"os/exec"
	"strconv"

	"github.com/deckhouse/virtualization/src/cli/pkg/usbredir"
)

type Redirector interface {
	Redirect(ctx context.Context, address string) error
}

type usbToolRedirector struct {
	bus       int
	deviceNum int
	verbosity int
	bin       string
	sudo      bool
}

func newUsbToolRedirector(bus, deviceNum, verbosity int, bin string, sudo bool) *usbToolRedirector {
	return &usbToolRedirector{
		bus:       bus,
		deviceNum: deviceNum,
		verbosity: verbosity,
		bin:       bin,
		sudo:      sudo,
	}
}

func (u usbToolRedirector) Redirect(ctx context.Context, address string) error {
	if _, err := exec.LookPath(u.bin); err != nil {
		return fmt.Errorf("error on finding %s in $PATH: %s", u.bin, err.Error())
	}

	var (
		command string
		args    []string
	)

	device := fmt.Sprintf("%d-%d", u.bus, u.deviceNum)

	if u.sudo {
		command = "sudo"
		args = []string{u.bin, "--device", device, "--to", address}
	} else {
		command = u.bin
		args = []string{"--device", device, "--to", address}
	}

	if u.verbosity > 0 {
		args = append(args, "--verbose", strconv.Itoa(u.verbosity))
	}

	output, err := exec.CommandContext(ctx, command, args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("failed to invoke usbredir: %w: %s", err, string(output))
	}
	return nil
}

func newNativeUsbRedirector(bus, deviceNum, verbosity int) *nativeUsbRedirector {
	return &nativeUsbRedirector{
		bus:       bus,
		deviceNum: deviceNum,
		verbosity: verbosity,
	}
}

type nativeUsbRedirector struct {
	bus       int
	deviceNum int
	verbosity int
}

func (u nativeUsbRedirector) Redirect(ctx context.Context, address string) error {
	host, portStr, err := net.SplitHostPort(address)
	if err != nil {
		return fmt.Errorf("invalid address: %w", err)
	}
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return fmt.Errorf("invalid port: %w", err)
	}

	config := usbredir.Config{
		Bus:       u.bus,
		DeviceNum: u.deviceNum,
		Address:   host,
		Port:      port,
		Verbosity: u.verbosity,
	}
	return usbredir.Run(ctx, config)
}
