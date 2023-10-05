package main

import (
	"flag"
	"os"
	"strconv"
	"strings"

	ocpcrypto "github.com/openshift/library-go/pkg/crypto"
	"k8s.io/klog/v2"
	"kubevirt.io/containerized-data-importer/pkg/common"
	cryptowatch "kubevirt.io/containerized-data-importer/pkg/util/tls-crypto-watch"

	"github.com/deckhouse/virtualization-controller/dvcr-importers/pkg/uploader"
)

const (
	defaultListenPort    = 8444
	defaultListenAddress = "0.0.0.0"
)

func init() {
	klog.InitFlags(nil)
	flag.Parse()
}

func main() {
	defer klog.Flush()

	listenAddress, listenPort := getListenAddressAndPort()

	cryptoConfig := getCryptoConfig()

	server, err := uploader.NewUploadServer(
		listenAddress,
		listenPort,
		os.Getenv("TLS_KEY"),
		os.Getenv("TLS_CERT"),
		os.Getenv("CLIENT_CERT"),
		os.Getenv("CLIENT_NAME"),
		cryptoConfig,
	)
	if err != nil {
		klog.Fatalf("UploadServer failed: %s", err)
	}

	klog.Infof("Running server on %s:%d", listenAddress, listenPort)

	err = server.Run()
	if err != nil {
		klog.Fatalf("UploadServer failed: %s", err)
	}

	klog.Info("UploadServer exited")
}

func getListenAddressAndPort() (string, int) {
	addr, port := defaultListenAddress, defaultListenPort

	// empty value okay here
	if val, exists := os.LookupEnv("LISTEN_ADDRESS"); exists {
		addr = val
	}

	// not okay here
	if val := os.Getenv("LISTEN_PORT"); len(val) > 0 {
		n, err := strconv.ParseUint(val, 10, 16)
		if err == nil {
			port = int(n)
		}
	}

	return addr, port
}

func getCryptoConfig() cryptowatch.CryptoConfig {
	ciphersNames := strings.Split(os.Getenv(common.CiphersTLSVar), ",")
	ciphers := cryptowatch.CipherSuitesIDs(ciphersNames)
	minTLSVersion, _ := ocpcrypto.TLSVersion(os.Getenv(common.MinVersionTLSVar))

	return cryptowatch.CryptoConfig{
		CipherSuites: ciphers,
		MinVersion:   minTLSVersion,
	}
}
