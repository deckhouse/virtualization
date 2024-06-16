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
