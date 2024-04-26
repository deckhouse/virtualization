//go:build linux
// +build linux

package netlinkwrap

import (
	orignetlink "github.com/vishvananda/netlink"
)

// Aliases for some netlink functions and constants available only for Linux.

const (
	FAMILY_ALL = orignetlink.FAMILY_ALL
)

var RuleAdd = orignetlink.RuleAdd

var RuleDel = orignetlink.RuleDel

var RuleListFiltered = orignetlink.RuleListFiltered
