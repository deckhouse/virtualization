/*
Copyright 2024 Flant JSC

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
