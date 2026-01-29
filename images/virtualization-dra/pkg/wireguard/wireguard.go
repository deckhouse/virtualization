package wireguard

import (
	"errors"
	"fmt"
	"net"
	"os"
	"slices"
	"time"

	"github.com/vishvananda/netlink"
	"golang.zx2c4.com/wireguard/wgctrl"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

const defaultPersistentKeepalive = 25 * time.Second

func generateKeys() (wgtypes.Key, wgtypes.Key, error) {
	privateKey, err := wgtypes.GeneratePrivateKey()
	if err != nil {
		return wgtypes.Key{}, wgtypes.Key{}, fmt.Errorf("failed to generate private key: %w", err)
	}

	publicKey := privateKey.PublicKey()

	return publicKey, privateKey, nil
}

type wgManager struct{}

func (m *wgManager) createDevice(iface string) error {
	link := &netlink.Wireguard{LinkAttrs: netlink.LinkAttrs{Name: iface}}
	err := netlink.LinkAdd(link)
	if err != nil {
		if errors.Is(err, os.ErrExist) {
			return nil
		}
		return fmt.Errorf("failed to create wireguard device: %w", err)
	}
	return nil
}

func (m *wgManager) ensureAddAddrAndLinkUp(iface string, ipNet *net.IPNet) error {
	link, err := netlink.LinkByName(iface)
	if err != nil {
		return fmt.Errorf("failed to get link %s: %w", iface, err)
	}

	if link.Attrs().OperState == netlink.OperUp {
		return nil
	}

	if err := netlink.AddrAdd(link, &netlink.Addr{IPNet: ipNet}); err != nil && !errors.Is(err, os.ErrExist) {
		return fmt.Errorf("failed to add address: %w", err)
	}

	if err = netlink.LinkSetUp(link); err != nil {
		return fmt.Errorf("failed to bring up wireguard device: %w", err)
	}

	return nil
}

func (m *wgManager) getDevice(iface string, client *wgctrl.Client) (*wgtypes.Device, bool, error) {
	dev, err := client.Device(iface)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, false, nil
		}
		return nil, false, fmt.Errorf("failed to get device: %w", err)
	}
	return dev, true, nil
}

func (m *wgManager) ConfigureDevice(iface string, ipNet *net.IPNet, config Config) error {
	client, err := wgctrl.New()
	if err != nil {
		return fmt.Errorf("failed to create wgctrl client: %w", err)
	}
	defer client.Close()

	dev, exist, err := m.getDevice(iface, client)
	if err != nil {
		return err
	}

	if !exist {
		if err = m.createDevice(iface); err != nil {
			return err
		}
		err = client.ConfigureDevice(iface, config.WGConfigWithReplace())
		if err != nil {
			return fmt.Errorf("failed to configure device: %w", err)
		}
		return m.ensureAddAddrAndLinkUp(iface, ipNet)
	}

	oldConfig := NewConfigFromDevice(dev)
	shouldConfigure := !config.Equal(oldConfig)
	if shouldConfigure {
		err = client.ConfigureDevice(iface, config.WGConfig(oldConfig))
		if err != nil {
			return fmt.Errorf("failed to configure device: %w", err)
		}
	}

	return m.ensureAddAddrAndLinkUp(iface, ipNet)
}

func NewConfigFromDevice(dev *wgtypes.Device) Config {
	peers := make([]Peer, len(dev.Peers))
	for i, peer := range dev.Peers {
		peers[i] = NewPeer(peer.PublicKey, *peer.Endpoint, peer.PersistentKeepaliveInterval, peer.AllowedIPs)
	}

	return Config{
		PrivateKey: dev.PrivateKey,
		PublicKey:  dev.PublicKey,
		ListenPort: dev.ListenPort,
		Peers:      peers,
	}
}

func NewConfig(port int, priv, pub wgtypes.Key, peers []Peer) Config {
	return Config{
		ListenPort: port,
		PrivateKey: priv,
		PublicKey:  pub,
		Peers:      peers,
	}
}

type Config struct {
	ListenPort int
	PrivateKey wgtypes.Key
	PublicKey  wgtypes.Key
	Peers      []Peer
}

func (c Config) Equal(other Config) bool {
	return c.ListenPort == other.ListenPort &&
		c.PrivateKey.String() == other.PrivateKey.String() &&
		c.PublicKey.String() == other.PublicKey.String() &&
		slices.EqualFunc(c.Peers, other.Peers, func(peer Peer, peer2 Peer) bool {
			return peer.Equal(peer2)
		})
}

func (c Config) WGConfigWithReplace() wgtypes.Config {
	peers := make([]wgtypes.PeerConfig, len(c.Peers))
	for i, peer := range c.Peers {
		peers[i] = peer.WGPeerConfig(false)
	}
	return wgtypes.Config{
		PrivateKey:   &c.PrivateKey,
		ListenPort:   &c.ListenPort,
		ReplacePeers: true,
		Peers:        peers,
	}
}

func (c Config) WGConfig(oldConfig Config) wgtypes.Config {
	return convertToWgConfig(oldConfig, c)
}

func NewPeer(publicKey wgtypes.Key, endpoint net.UDPAddr, persistentKeepaliveInterval time.Duration, allowedIPs []net.IPNet) Peer {
	return Peer{
		PublicKey:                   publicKey,
		Endpoint:                    endpoint,
		PersistentKeepaliveInterval: persistentKeepaliveInterval,
		AllowedIPs:                  allowedIPs,
	}
}

type Peer struct {
	PublicKey                   wgtypes.Key
	Endpoint                    net.UDPAddr
	PersistentKeepaliveInterval time.Duration
	AllowedIPs                  []net.IPNet
}

func (p Peer) Equal(other Peer) bool {
	return p.PublicKey.String() == other.PublicKey.String() &&
		p.Endpoint.String() == other.Endpoint.String() &&
		p.PersistentKeepaliveInterval == other.PersistentKeepaliveInterval &&
		slices.EqualFunc(p.AllowedIPs, other.AllowedIPs, func(ipNet net.IPNet, ipNet2 net.IPNet) bool {
			return ipNet.String() == ipNet2.String()
		})
}

func (p Peer) WGPeerConfig(removed bool) wgtypes.PeerConfig {
	if removed {
		return wgtypes.PeerConfig{
			PublicKey: p.PublicKey,
			Remove:    true,
		}
	}
	return wgtypes.PeerConfig{
		PublicKey:                   p.PublicKey,
		UpdateOnly:                  true,
		Endpoint:                    &p.Endpoint,
		PersistentKeepaliveInterval: &p.PersistentKeepaliveInterval,
		ReplaceAllowedIPs:           true,
		AllowedIPs:                  p.AllowedIPs,
	}
}

func convertToWgConfig(oldConfig, newConfig Config) wgtypes.Config {
	wgConfig := wgtypes.Config{
		PrivateKey: &newConfig.PrivateKey,
		ListenPort: &newConfig.ListenPort,
	}

	oldPeers := make(map[string]Peer)
	for _, peer := range oldConfig.Peers {
		oldPeers[peer.PublicKey.String()] = peer
	}

	var peerConfigs []wgtypes.PeerConfig

	for _, newPeer := range newConfig.Peers {
		oldPeer, exist := oldPeers[newPeer.PublicKey.String()]

		shouldUpdate := !exist || !newPeer.Equal(oldPeer)
		if shouldUpdate {
			peerConfigs = append(peerConfigs, newPeer.WGPeerConfig(false))
		}
		delete(oldPeers, newPeer.PublicKey.String())
	}

	for _, oldPeer := range oldPeers {
		peerConfigs = append(peerConfigs, oldPeer.WGPeerConfig(true))
	}

	wgConfig.Peers = peerConfigs

	return wgConfig
}
