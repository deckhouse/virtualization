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

package main

import (
	"flag"
	"os"
	"strconv"

	"k8s.io/klog/v2"

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

	server, err := uploader.NewUploadServer(
		listenAddress,
		listenPort,
		os.Getenv("TLS_KEY"),
		os.Getenv("TLS_CERT"),
		os.Getenv("CLIENT_CERT"),
		os.Getenv("CLIENT_NAME"),
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
