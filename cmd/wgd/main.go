package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"github.com/pkg/errors"
	"github.com/sirupsen/logrus"
	"golang.zx2c4.com/wireguard/wgctrl"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"time"
	"wfis.lol/wgd/daemon"
	"wfis.lol/wgd/libs/iface"
)

const IfaceName = "wg0"

type WgDaemon struct {
	client *wgctrl.Client

	apiKey   string
	masterIp string

	address    net.IPNet
	port       int
	privateKey wgtypes.Key

	configVersion int

	logger *logrus.Entry
}

func main() {
	var masterIp string
	var apiKey string

	flag.StringVar(&masterIp, "masterIp", "", "master ip to register daemon at")
	flag.StringVar(&apiKey, "apiKey", "", "api key to register daemon")
	flag.Parse()

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	logger := daemon.CreateLogger()

	client, err := wgctrl.New()

	if err != nil {
		logger.WithError(err).Fatal("failed to open client")
	}

	d := &WgDaemon{
		client: client,

		apiKey:   apiKey,
		masterIp: masterIp,

		logger: logger,
	}

	err = d.configDaemon(ctx)

	if err != nil {
		logger.WithError(err).Fatal("failed to register")
	}

	// heartbeat
	go d.startHeartbeat(ctx)

	// if interface exists, delete it
	if exists, err := d.interfaceExists(IfaceName); err != nil {
		logger.WithError(err).Fatal("failed to check if device exists")
	} else if exists {
		logger.Warnf("interface %s already exists, deleting...", IfaceName)

		if err := iface.Delete(IfaceName); err != nil {
			logger.WithError(err).Fatal("failed to delete interface")
		}
	}

	// create interface
	err = iface.Create(IfaceName, d.address.String())
	if err != nil {
		logger.WithError(err).Fatal("failed to create interface")
	}

	go d.listenForConfigChanges(ctx, IfaceName)

	<-ctx.Done()
}

type WgDaemonConfigDto struct {
	PrivateKey string `json:"privateKey"`
	Address    string `json:"address"`
	Port       int    `json:"port"`
}

func (d *WgDaemon) getConfigVersion() (int, error) {
	req, err := http.NewRequest("GET", fmt.Sprintf("http://%s/api/daemon/config/version", d.masterIp), nil)

	if err != nil {
		return 0, errors.Wrap(err, "failed to create request")
	}

	req.Header.Set("X-Daemon-Key", d.apiKey)

	client := http.Client{}

	resp, err := client.Do(req)

	if err != nil {
		return 0, errors.Wrap(err, "failed to send request")
	}

	if resp.StatusCode != http.StatusOK {
		return 0, errors.New("invalid response")
	}

	body, err := io.ReadAll(resp.Body)

	if err != nil {
		return 0, errors.Wrap(err, "failed to read response")
	}

	version, err := strconv.Atoi(string(body))

	if err != nil {
		return 0, errors.Wrap(err, "failed to parse version")
	}

	return version, nil
}

type WgDaemonPeerConfigDto struct {
	PublicKey  string   `json:"publicKey"`
	Endpoint   string   `json:"endpoint"`
	Port       int      `json:"port"`
	AllowedIps []string `json:"allowedIps"`
}

func (d *WgDaemon) configDaemon(ctx context.Context) error {
	config, err := d.getDaemonConfig(ctx)

	if err != nil {
		return errors.Wrap(err, "failed to get daemon config")
	}

	_, address, err := net.ParseCIDR(config.Address)

	if err != nil {
		return errors.Wrap(err, "failed to parse address")
	}

	wireguardPrivateKey, err := wgtypes.ParseKey(config.PrivateKey)

	if err != nil {
		return errors.Wrap(err, "failed to parse private key")
	}

	port := config.Port

	d.address = *address
	d.port = port
	d.privateKey = wireguardPrivateKey

	return nil
}

func (d *WgDaemon) getDaemonConfig(ctx context.Context) (*WgDaemonConfigDto, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", fmt.Sprintf("http://%s/api/daemon/config", d.masterIp), nil)

	if err != nil {
		return nil, errors.Wrap(err, "failed to create request")
	}

	req.Header.Set("X-Daemon-Key", d.apiKey)

	client := http.Client{}

	resp, err := client.Do(req)

	if err != nil {
		return nil, errors.Wrap(err, "failed to send request")
	}

	if resp.StatusCode != http.StatusOK {
		return nil, errors.New("failed to send request")
	}

	config := WgDaemonConfigDto{}

	err = json.NewDecoder(resp.Body).Decode(&config)

	if err != nil {
		return nil, errors.Wrap(err, "failed to decode response")
	}

	return &config, nil
}

func (d *WgDaemon) startHeartbeat(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
			err := d.sendHeartbeat(ctx)

			if err != nil {
				d.logger.WithError(err).Error("failed to send heartbeat")
			}

			<-time.After(10 * time.Second)
		}
	}
}

func (d *WgDaemon) sendHeartbeat(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, "GET", fmt.Sprintf("http://%s/api/daemon/heartbeat", d.masterIp), nil)

	if err != nil {
		return errors.Wrap(err, "failed to create request")
	}

	req.Header.Set("X-Daemon-Key", d.apiKey)

	client := http.Client{}

	resp, err := client.Do(req)

	if err != nil {
		return errors.Wrap(err, "failed to send request")
	}

	if resp.StatusCode != http.StatusOK {
		return errors.New("invalid response")
	}

	return nil
}

func (d *WgDaemon) listenForConfigChanges(ctx context.Context, ifName string) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
			version, err := d.getConfigVersion()

			if err != nil {
				d.logger.WithError(err).Error("failed to get config version")
				<-time.After(10 * time.Second)
				continue
			}

			if version != d.configVersion {
				d.logger.Info("config version changed, updating config")

				peers, err := d.getPeers(ctx)

				if err != nil {
					d.logger.WithError(err).Error("failed to update config")
				}

				err = iface.FlushRoutes(ifName)

				if err != nil {
					d.logger.WithError(err).Error("failed to clear routing")
				}

				err = d.applyPeers(peers)

				if err != nil {
					d.logger.WithError(err).Error("failed to apply peers")
				}

				for _, peer := range peers {
					for _, allowedIp := range peer.AllowedIps {
						_, ipNet, err := net.ParseCIDR(allowedIp)

						if err != nil {
							d.logger.WithError(err).Error("failed to parse allowed ip")
							continue
						}

						err = iface.SetRoute(ifName, ipNet)
						if err != nil {
							d.logger.WithError(err).Error("failed to add route")
							continue
						}
					}
				}

				d.configVersion = version
			}

			<-time.After(1 * time.Minute)
		}
	}
}

func (d *WgDaemon) getPeers(ctx context.Context) ([]WgDaemonPeerConfigDto, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", fmt.Sprintf("http://%s/api/daemon/peers", d.masterIp), nil)

	if err != nil {
		return nil, errors.Wrap(err, "failed to create request")
	}

	req.Header.Set("X-Daemon-Key", d.apiKey)

	client := http.Client{}

	resp, err := client.Do(req)

	if err != nil {
		return nil, errors.Wrap(err, "failed to send request")
	}

	if resp.StatusCode != http.StatusOK {
		return nil, errors.New("invalid response")
	}

	var peers []WgDaemonPeerConfigDto

	err = json.NewDecoder(resp.Body).Decode(&peers)

	if err != nil {
		return nil, errors.Wrap(err, "failed to decode response")
	}

	return peers, nil
}

func (d *WgDaemon) applyPeers(peers []WgDaemonPeerConfigDto) error {
	var peerConfigs []wgtypes.PeerConfig

	for _, peer := range peers {
		publicKey, err := wgtypes.ParseKey(peer.PublicKey)

		if err != nil {
			return errors.Wrap(err, "failed to parse public key")
		}

		var endpoint *net.UDPAddr

		if peer.Endpoint != "" && peer.Port != 0 {
			ep := net.ParseIP(peer.Endpoint)

			endpoint = &net.UDPAddr{
				IP:   ep,
				Port: peer.Port,
			}
		}

		var allowedIps []net.IPNet

		for _, allowedIp := range peer.AllowedIps {
			_, ipNet, err := net.ParseCIDR(allowedIp)

			if err != nil {
				return errors.Wrap(err, "failed to parse allowed ip")
			}

			allowedIps = append(allowedIps, *ipNet)
		}

		peerConfigs = append(peerConfigs, wgtypes.PeerConfig{
			PublicKey:  publicKey,
			Endpoint:   endpoint,
			AllowedIPs: allowedIps,
		})
	}

	err := d.client.ConfigureDevice(IfaceName, wgtypes.Config{
		PrivateKey:   &d.privateKey,
		ListenPort:   &d.port,
		ReplacePeers: true,
		Peers:        peerConfigs,
	})

	if err != nil {
		return errors.Wrap(err, "failed to configure device")
	}

	return nil
}

func (d *WgDaemon) interfaceExists(ifName string) (bool, error) {
	devices, err := d.client.Devices()

	if err != nil {
		return false, err
	}

	for _, device := range devices {
		if device.Name == ifName {
			return true, nil
		}
	}

	return false, nil
}
