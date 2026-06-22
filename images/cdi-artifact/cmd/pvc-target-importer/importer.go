package main

import (
	"flag"
	"fmt"
	"net"
	"net/url"
	"os"
	"time"

	"k8s.io/klog/v2"

	"kubevirt.io/containerized-data-importer/pkg/common"
	"kubevirt.io/containerized-data-importer/pkg/image"
	"kubevirt.io/containerized-data-importer/pkg/util"
	prometheusutil "kubevirt.io/containerized-data-importer/pkg/util/prometheus"
)

const (
	completeMessage = "Import Complete"

	nbdConnectTimeout = 10 * time.Minute
	nbdDialInterval   = time.Second
)

func init() {
	klog.InitFlags(nil)
	flag.Parse()
}

func main() {
	defer klog.Flush()

	certsDirectory, err := os.MkdirTemp("", "certsdir")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(certsDirectory)
	prometheusutil.StartPrometheusEndpoint(certsDirectory)

	nbdEndpoint, err := util.ParseEnvVar(common.ImporterNBDEndpoint, true)
	if err != nil {
		klog.Error(err)
		os.Exit(1)
	}

	dest := importerDestPath()
	if err := waitForNBDEndpoint(nbdEndpoint, nbdConnectTimeout); err != nil {
		klog.Errorf("%+v", err)
		if writeErr := util.WriteTerminationMessage(fmt.Sprintf("Unable to connect to NBD source: %v", err.Error())); writeErr != nil {
			klog.Errorf("%+v", writeErr)
		}
		os.Exit(1)
	}

	if err := image.ConvertNBDToRaw(nbdEndpoint, dest); err != nil {
		klog.Errorf("%+v", err)
		if writeErr := util.WriteTerminationMessage(fmt.Sprintf("Unable to convert NBD image: %v", err.Error())); writeErr != nil {
			klog.Errorf("%+v", writeErr)
		}
		os.Exit(1)
	}

	defer fsyncDataFile(dest)
	if err := writeTerminationMessage(completeMessage); err != nil {
		klog.Errorf("%+v", err)
		os.Exit(1)
	}
}

func importerDestPath() string {
	if _, err := os.Stat(common.WriteBlockPath); err == nil {
		return common.WriteBlockPath
	}
	return common.ImporterWritePath
}

func waitForNBDEndpoint(nbdEndpoint string, timeout time.Duration) error {
	parsed, err := url.Parse(nbdEndpoint)
	if err != nil {
		return fmt.Errorf("parse NBD endpoint %q: %w", nbdEndpoint, err)
	}
	if parsed.Scheme != "nbd" {
		return fmt.Errorf("unsupported NBD endpoint scheme %q", parsed.Scheme)
	}
	if parsed.Host == "" {
		return fmt.Errorf("NBD endpoint %q has empty host", nbdEndpoint)
	}

	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", parsed.Host, 2*time.Second)
		if err == nil {
			conn.Close()
			return nil
		}
		lastErr = err
		time.Sleep(nbdDialInterval)
	}
	return fmt.Errorf("timed out waiting for NBD endpoint %q: %w", nbdEndpoint, lastErr)
}

func writeTerminationMessage(message string) error {
	termMsg := &common.TerminationMessage{Message: &message}
	msg, err := termMsg.String()
	if err != nil {
		return err
	}
	if err := util.WriteTerminationMessage(msg); err != nil {
		return err
	}
	klog.V(1).Infoln(msg)
	return nil
}

func fsyncDataFile(path string) {
	file, err := os.Open(path)
	if err != nil {
		klog.Errorf("could not get file descriptor for fsync call: %+v", err)
		os.Exit(1)
	}
	if err := file.Sync(); err != nil {
		klog.Errorf("could not fsync following qemu-img writing: %+v", err)
		os.Exit(1)
	}
	klog.V(3).Infof("Successfully completed fsync(%s) syscall, committed to disk\n", path)
	file.Close()
}
