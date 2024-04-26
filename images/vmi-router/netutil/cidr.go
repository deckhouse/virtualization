package netutil

import "strings"

type CIDRSet []string

func (f *CIDRSet) String() string { return "" }
func (f *CIDRSet) Set(s string) error {
	*f = append(*f, s)
	return nil
}

const (
	hostNetmaskIPv4 = "/32"
	hostNetmaskIPv6 = "/128"
)

func AppendHostNetmask(ip string) string {
	if strings.Contains(ip, "/") {
		// IP already contains netmask
		return ip
	}
	if strings.Contains(ip, ":") {
		// IPv6
		return ip + hostNetmaskIPv6
	}
	// IPv4
	return ip + hostNetmaskIPv4
}
