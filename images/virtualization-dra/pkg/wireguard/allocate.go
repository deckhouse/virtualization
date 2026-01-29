package wireguard

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/tools/cache"

	vdraapi "github.com/deckhouse/virtualization-dra/api/v1alpha1"
)

func (c *Controller) AllocateAddress(ctx context.Context) (string, error) {
	if !cache.WaitForCacheSync(ctx.Done(), c.hasSynced) {
		return "", fmt.Errorf("failed to wait for caches to sync")
	}
	return c.tryAllocateAddress(ctx)
}

func (c *Controller) tryAllocateAddress(ctx context.Context) (string, error) {
	var address string

	allocCtx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()

	startTime := time.Now()
	attempt := 0

	err := wait.PollUntilContextCancel(allocCtx, 5*time.Second, false, func(ctx context.Context) (done bool, err error) {
		attempt++
		elapsed := time.Since(startTime)

		if attempt%6 == 0 {
			c.log.Info("Still trying to allocate address...",
				slog.Int("attempt", attempt),
				slog.Duration("elapsed", elapsed))
		}

		wsn, err := c.getWireguardSystemNetwork()
		if err != nil {
			c.log.Warn("Failed to get WireguardSystemNetwork",
				slog.Any("error", err),
				slog.Int("attempt", attempt))
			return false, nil
		}

		for _, alloc := range wsn.Status.AllocatedIPs {
			if alloc.Node == c.nodeName {
				address = alloc.IP
				c.log.Info("Found existing allocated address",
					slog.String("address", address),
					slog.Duration("elapsed", elapsed))
				return true, nil
			}
		}

		c.log.Info("Allocating new address", slog.Int("attempt", attempt))

		allocAddr, err := c.allocateAddr(wsn)
		if err != nil {
			c.log.Error("Failed to allocate address",
				slog.Any("error", err),
				slog.String("node", c.nodeName))
			return false, nil
		}

		wsn.Status.AllocatedIPs = append(wsn.Status.AllocatedIPs, vdraapi.AllocatedIP{
			Node: c.nodeName,
			IP:   allocAddr,
		})

		_, err = c.vdraClient.WireguardSystemNetworks(wsn.Namespace).UpdateStatus(ctx, wsn, metav1.UpdateOptions{})

		if err != nil {
			c.log.Error("Failed to apply update status",
				slog.Any("error", err),
				slog.String("address", allocAddr))
			return false, nil
		}

		address = allocAddr
		c.log.Info("Successfully allocated address",
			slog.String("address", address),
			slog.Int("attempts", attempt),
			slog.Duration("elapsed", elapsed))

		return true, nil
	})

	elapsed := time.Since(startTime)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			return "", fmt.Errorf("address allocation timeout after %v (attempts: %d)", elapsed, attempt)
		}
		return "", fmt.Errorf("address allocation failed after %v (attempts: %d): %w", elapsed, attempt, err)
	}

	if address == "" {
		return "", fmt.Errorf("address allocation failed after %v (attempts: %d)", elapsed, attempt)
	}

	return address, nil
}

func (c *Controller) allocateAddr(wsn *vdraapi.WireguardSystemNetwork) (string, error) {
	_, ipv4Net, err := net.ParseCIDR(wsn.Spec.CIDR)
	if err != nil {
		return "", fmt.Errorf("failed to parse CIDR: %w", err)
	}

	allocated := make(map[string]struct{})
	for _, alloc := range wsn.Status.AllocatedIPs {
		allocated[alloc.IP] = struct{}{}
	}

	for ip := c.incIP(ipv4Net.IP.Mask(ipv4Net.Mask)); ipv4Net.Contains(ip); ip = c.incIP(ip) {
		ipStr := ip.String()
		if _, ok := allocated[ipStr]; !ok {
			if !c.isBroadcast(ip, ipv4Net) {
				return ipStr, nil
			}
		}
	}

	return "", fmt.Errorf("no available IP addresses")
}

func (c *Controller) incIP(ip net.IP) net.IP {
	ip = ip.To4()
	result := make(net.IP, 4)
	copy(result, ip)

	for i := 3; i >= 0; i-- {
		result[i]++
		if result[i] != 0 {
			break
		}
	}
	return result
}

func (c *Controller) isBroadcast(ip net.IP, ipnet *net.IPNet) bool {
	ip = ip.To4()
	for i := 0; i < 4; i++ {
		if ip[i] != (ipnet.IP.To4()[i] | ^ipnet.Mask[i]) {
			return false
		}
	}
	return true
}
