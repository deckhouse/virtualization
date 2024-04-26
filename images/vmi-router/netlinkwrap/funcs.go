package netlinkwrap

import (
	"net"

	orignetlink "github.com/vishvananda/netlink"
)

type Funcs struct {
	RouteGet          func(net.IP) ([]orignetlink.Route, error)
	RouteDel          func(*orignetlink.Route) error
	RouteReplace      func(*orignetlink.Route) error
	RouteListFiltered func(int, *orignetlink.Route, uint64) ([]orignetlink.Route, error)

	RuleAdd          func(*orignetlink.Rule) error
	RuleDel          func(*orignetlink.Rule) error
	RuleListFiltered func(int, *orignetlink.Rule, uint64) ([]orignetlink.Rule, error)
}

func NewFuncs() *Funcs {
	return &Funcs{
		// Rule methods are OS dependent.
		RuleAdd:          RuleAdd,
		RuleDel:          RuleDel,
		RuleListFiltered: RuleListFiltered,
		// Route methods are available for all OSes.
		RouteGet:          orignetlink.RouteGet,
		RouteDel:          orignetlink.RouteDel,
		RouteReplace:      orignetlink.RouteReplace,
		RouteListFiltered: orignetlink.RouteListFiltered,
	}
}

func DryRunFuncs() *Funcs {
	return &Funcs{
		// Rule methods are OS dependent.
		RuleAdd:          func(*orignetlink.Rule) error { return nil },
		RuleDel:          func(*orignetlink.Rule) error { return nil },
		RuleListFiltered: RuleListFiltered,
		// Route methods are available for all OSes.
		RouteGet:          orignetlink.RouteGet,
		RouteDel:          func(*orignetlink.Route) error { return nil },
		RouteReplace:      func(*orignetlink.Route) error { return nil },
		RouteListFiltered: orignetlink.RouteListFiltered,
	}
}
