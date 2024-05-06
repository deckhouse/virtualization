//go:build !linux
// +build !linux

package netlinkwrap

import (
	orignetlink "github.com/vishvananda/netlink"
)

// Aliases for netlink functions and constants not available for non-Linux.

const (
	FAMILY_ALL = 0
)

var RuleAdd = func(*orignetlink.Rule) error { return orignetlink.ErrNotImplemented }

var RuleDel = func(*orignetlink.Rule) error { return orignetlink.ErrNotImplemented }

var RuleListFiltered = func(int, *orignetlink.Rule, uint64) ([]orignetlink.Rule, error) {
	return nil, orignetlink.ErrNotImplemented
}
