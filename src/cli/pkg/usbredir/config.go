package usbredir

import "errors"

type Config struct {
	Vendor    int
	Product   int
	Bus       int
	DeviceNum int
	Port      int
	Verbosity int
	KeepAlive bool
	Address   string
}

func (c Config) Validate() error {
	if c.Address == "" {
		return errors.New("address is required")
	}
	if c.Port <= 0 || c.Port > 65535 {
		return errors.New("port must be between 1 and 65535")
	}

	byBus := c.Bus != 0 && c.DeviceNum != 0
	byVendor := c.Vendor != 0 && c.Product != 0

	if !byBus && !byVendor {
		return errors.New("either (bus,deviceNum) or (vendor,product) must be set")
	}

	return nil
}
