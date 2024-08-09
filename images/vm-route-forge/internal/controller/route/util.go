package route

import (
	"fmt"
	"net"
)

func isManagedIP(ip net.IP, cidrs []*net.IPNet) (bool, error) {
	if len(ip) == 0 {
		return false, fmt.Errorf("invalid IP address %s", ip)
	}

	for _, cidr := range cidrs {
		if cidr.Contains(ip) {
			return true, nil
		}
	}

	return false, nil
}
