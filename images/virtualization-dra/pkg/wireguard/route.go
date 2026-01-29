package wireguard

import (
	"fmt"
	"net"

	"github.com/vishvananda/netlink"
)

const DefaultRouteTableID = 1001

func newRouteManager(routeTableID int) *routeManager {
	if routeTableID == 0 {
		routeTableID = DefaultRouteTableID
	}
	return &routeManager{
		routeTableID: routeTableID,
	}
}

type routeManager struct {
	routeTableID int
}

func (m routeManager) Sync(iface string, cidrs []*net.IPNet) error {
	link, err := netlink.LinkByName(iface)
	if err != nil {
		return fmt.Errorf("cannot find iface %s: %w", iface, err)
	}

	desired := make(map[string]*net.IPNet)
	for _, cidr := range cidrs {
		desired[cidr.String()] = cidr
	}

	current, err := netlink.RouteListFiltered(
		netlink.FAMILY_ALL,
		&netlink.Route{
			Table:     m.routeTableID,
			LinkIndex: link.Attrs().Index,
		},
		netlink.RT_FILTER_TABLE|netlink.RT_FILTER_OIF,
	)
	if err != nil {
		return fmt.Errorf("failed to list routes: %w", err)
	}

	// Delete routes that are not in the desired list.
	for _, r := range current {
		if r.Dst == nil {
			continue
		}
		if _, ok := desired[r.Dst.String()]; !ok {
			if err := netlink.RouteDel(&r); err != nil {
				return fmt.Errorf("failed to delete route %s: %w", r.Dst, err)
			}
		}
	}

	// Add routes that are in the desired list.
	for _, dst := range desired {
		route := netlink.Route{
			LinkIndex: link.Attrs().Index,
			Dst:       dst,
			Table:     m.routeTableID,
			Scope:     netlink.SCOPE_LINK,
		}

		if err := netlink.RouteReplace(&route); err != nil {
			return fmt.Errorf("failed to add route %s: %w", dst, err)
		}
	}

	return nil
}
