package wireguard

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"slices"
	"strconv"
	"time"

	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
	"k8s.io/apimachinery/pkg/api/equality"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	vdraapi "github.com/deckhouse/virtualization-dra/api/v1alpha1"
	"github.com/deckhouse/virtualization-dra/pkg/patch"
)

func (c *Controller) getMyKeysAndEndpoint(wsn *vdraapi.WireguardSystemNetwork) (*wgtypes.Key, *wgtypes.Key, string) {
	for _, settings := range wsn.Status.NodeSettings {
		if settings.Node == c.nodeName {
			var priv, pub *wgtypes.Key

			if settings.PrivateKey != "" {
				if key, err := wgtypes.ParseKey(settings.PrivateKey); err == nil {
					priv = &key
				}

			}
			if settings.PublicKey != "" {
				if key, err := wgtypes.ParseKey(settings.PublicKey); err == nil {
					pub = &key
				}

			}

			return priv, pub, settings.Endpoint
		}
	}
	return nil, nil, ""
}

func (c *Controller) updateNodeSettingsIfNeeded(ctx context.Context, wsn *vdraapi.WireguardSystemNetwork) error {
	privateKey, publicKey, _ := c.getMyKeysAndEndpoint(wsn)

	shouldRegenerateAll := privateKey == nil
	shouldRegeneratePublic := publicKey == nil

	switch {
	case shouldRegenerateAll:
		c.log.Info("Regenerating all keys for this node")
		priv, pub, err := generateKeys()
		if err != nil {
			return err
		}
		privateKey = &priv
		publicKey = &pub
	case shouldRegeneratePublic:
		c.log.Info("Regenerating public key for this node")
		pub := privateKey.PublicKey()
		publicKey = &pub
	}

	newNodeSettings := c.newNodeSettings(*privateKey, *publicKey, wsn.Spec)

	indx := slices.IndexFunc(wsn.Status.NodeSettings, func(settings vdraapi.NodeSettings) bool {
		return settings.Node == c.nodeName
	})

	if indx == -1 {
		statusCopy := wsn.Status.DeepCopy()
		statusCopy.NodeSettings = append(statusCopy.NodeSettings, newNodeSettings)

		p := patch.NewJSONPatch(patch.WithTest("/status/nodeSettings", wsn.Status.NodeSettings))
		if wsn.Status.NodeSettings == nil {
			p.Append(patch.WithAdd("/status/nodeSettings", statusCopy.NodeSettings))
		} else {
			p.Append(patch.WithReplace("/status/nodeSettings", statusCopy.NodeSettings))
		}

		b, err := p.Bytes()
		if err != nil {
			return err
		}

		newWsn, err := c.vdraClient.WireguardSystemNetworks(c.namespace).Patch(ctx, wsn.Name, types.JSONPatchType, b, metav1.PatchOptions{}, "status")
		if err != nil {
			return fmt.Errorf("failed to patch WireguardSystemNetwork: %w", err)
		}

		*wsn = *newWsn

		return nil
	}

	oldNodeSettings := wsn.Status.NodeSettings[indx]

	if equality.Semantic.DeepEqual(oldNodeSettings, newNodeSettings) {
		return nil
	}

	path := fmt.Sprintf("/status/nodeSettings/%d", indx)

	b, err := patch.NewJSONPatch(
		patch.WithTest(path, oldNodeSettings),
		patch.WithReplace(path, newNodeSettings)).
		Bytes()
	if err != nil {
		return err
	}

	newWsn, err := c.vdraClient.WireguardSystemNetworks(c.namespace).Patch(ctx, wsn.Name, types.JSONPatchType, b, metav1.PatchOptions{}, "status")
	if err != nil {
		return fmt.Errorf("failed to patch WireguardSystemNetwork: %w", err)
	}

	*wsn = *newWsn

	return nil
}

func (c *Controller) newNodeSettings(priv, pub wgtypes.Key, wsnSpec vdraapi.WireguardSystemNetworkSpec) vdraapi.NodeSettings {
	return vdraapi.NodeSettings{
		Node:       c.nodeName,
		PrivateKey: priv.String(),
		PublicKey:  pub.String(),
		Endpoint:   net.JoinHostPort(c.podIP, strconv.Itoa(wsnSpec.ListenPort)),
	}
}

func (c *Controller) getPeers(wsn *vdraapi.WireguardSystemNetwork) ([]Peer, error) {
	var peers []Peer

	_, ipv4Net, err := net.ParseCIDR(wsn.Spec.CIDR)
	if err != nil {
		return nil, fmt.Errorf("failed to parse CIDR: %w", err)
	}

	for _, settings := range wsn.Status.NodeSettings {
		if settings.Node == c.nodeName {
			continue
		}
		pub, err := wgtypes.ParseKey(settings.PublicKey)
		if err != nil {
			c.log.Warn("failed to parse public key peer, skip...", slog.String("publicKey", settings.PublicKey), slog.Any("error", err), slog.String("node", settings.Node))
			continue
		}
		endpoint, err := c.getUDPEndpointFromString(settings.Endpoint)
		if err != nil {
			c.log.Warn("failed to parse endpoint peer, skip...", slog.String("endpoint", settings.Endpoint), slog.Any("error", err), slog.String("node", settings.Node))
			continue
		}

		pkeep := time.Duration(wsn.Spec.PersistentKeepalive) * time.Second
		if pkeep == 0 {
			pkeep = defaultPersistentKeepalive
		}

		peers = append(peers, NewPeer(pub, endpoint, pkeep, []net.IPNet{*ipv4Net}))
	}

	return peers, nil
}

func (c *Controller) getUDPEndpointFromString(endpoint string) (net.UDPAddr, error) {
	host, port, err := net.SplitHostPort(endpoint)
	if err != nil {
		return net.UDPAddr{}, fmt.Errorf("failed to parse endpoint: %w", err)
	}
	portInt, err := strconv.Atoi(port)
	if err != nil {
		return net.UDPAddr{}, fmt.Errorf("failed to parse port: %w", err)
	}
	return net.UDPAddr{IP: net.ParseIP(host), Port: portInt}, nil
}
