/*
Copyright 2018 The KubeVirt Authors.
Copyright 2024 Flant JSC.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.

Initially copied from https://github.com/kubevirt/kubevirt/blob/main/pkg/virtctl/vnc/vnc.go
*/

package vnc

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"time"

	"github.com/golang/glog"
	"github.com/spf13/cobra"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/deckhouse/virtualization-controller/api/client/kubecli"
	"github.com/deckhouse/virtualization-controller/pkg/d8vctl/templates"
)

const (
	ListenTimeout = 60 * time.Second

	// MACOSTigerVNCPATTERN
	// #### Tiger VNC ####
	// https://github.com/TigerVNC/tigervnc/releases
	// Compatible with multiple Tiger VNC versions
	MACOSTigerVNCPATTERN = `/Applications/TigerVNC Viewer*.app/Contents/MacOS/TigerVNC Viewer`

	// MACOSChickenVNC #### Chicken VNC ####
	//# https://sourceforge.net/projects/chicken/
	MACOSChickenVNC = "/Applications/Chicken.app/Contents/MacOS/Chicken"

	// MACOSRealVNC
	// #### Real VNC ####
	// https://www.realvnc.com/en/connect/download/viewer/macos/
	MACOSRealVNC = "/Applications/VNC Viewer.app/Contents/MacOS/vncviewer"

	RemoteViewer = "remote-viewer"
	TigerVNC     = "vncviewer"
)

var listenAddressFmt string

var (
	listenAddress = "127.0.0.1"
	proxyOnly     bool
	customPort    = 0
)

func NewCommand(clientConfig clientcmd.ClientConfig) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "vnc (VMI)",
		Short:   "Open a vnc connection to a virtual machine.",
		Example: usage(),
		Args:    templates.ExactArgs("vnc", 1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c := VNC{clientConfig: clientConfig}
			return c.Run(cmd, args)
		},
	}
	cmd.Flags().StringVar(&listenAddress, "address", listenAddress, "--address=127.0.0.1: Setting this will change the listening address of the VNC server. Example: --address=0.0.0.0 will make the server listen on all interfaces.")
	cmd.Flags().BoolVar(&proxyOnly, "proxy-only", proxyOnly, "--proxy-only=false: Setting this true will run only the d8vctl vnc proxy and show the port where VNC viewers can connect")
	cmd.Flags().IntVar(&customPort, "port", customPort,
		"--port=0: Assigning a port value to this will try to run the proxy on the given port if the port is accessible; If unassigned, the proxy will run on a random port")
	cmd.SetUsageTemplate(templates.UsageTemplate())
	return cmd
}

type VNC struct {
	clientConfig clientcmd.ClientConfig
}

func (o *VNC) Run(cmd *cobra.Command, args []string) error {
	namespace, _, err := o.clientConfig.Namespace()
	if err != nil {
		return err
	}

	vm := args[0]

	virtCli, err := kubecli.GetClientFromClientConfig(o.clientConfig)
	if err != nil {
		return err
	}

	// setup connection with VM
	vnc, err := virtCli.VirtualMachines(namespace).VNC(vm)
	if err != nil {
		return fmt.Errorf("Can't access VM %s: %s", vm, err.Error())
	}
	// Format the listening address to account for the port (ex: 127.0.0.0:5900)
	// Set listenAddress to localhost if proxy-only flag is not set
	if !proxyOnly {
		listenAddress = "127.0.0.1"
		glog.V(2).Infof("--proxy-only is set to false, listening on %s\n", listenAddress)
	}
	listenAddressFmt = listenAddress + ":%d"
	lnAddr, err := net.ResolveTCPAddr("tcp", fmt.Sprintf(listenAddressFmt, customPort))
	if err != nil {
		return fmt.Errorf("Can't resolve the address: %s", err.Error())
	}

	// The local tcp server is used to proxy the podExec websock connection to vnc client
	ln, err := net.ListenTCP("tcp", lnAddr)
	if err != nil {
		return fmt.Errorf("Can't listen on unix socket: %s", err.Error())
	}
	// End of pre-flight checks. Everything looks good, we can start
	// the goroutines and let the data flow

	//                                       -> pipeInWriter  -> pipeInReader
	// remote-viewer -> unix sock connection
	//                                       <- pipeOutReader <- pipeOutWriter
	pipeInReader, pipeInWriter := io.Pipe()
	pipeOutReader, pipeOutWriter := io.Pipe()

	k8ResChan := make(chan error)
	listenResChan := make(chan error)
	viewResChan := make(chan error)
	stopChan := make(chan struct{}, 1)
	doneChan := make(chan struct{}, 1)
	writeStop := make(chan error)
	readStop := make(chan error)

	go func() {
		// transfer data from/to the VM
		k8ResChan <- vnc.Stream(kubecli.StreamOptions{
			In:  pipeInReader,
			Out: pipeOutWriter,
		})
	}()

	// wait for vnc client to connect to our local proxy server
	go func() {
		start := time.Now()
		glog.Infof("connection timeout: %v", ListenTimeout)
		// Don't set deadline if only proxy is running and VNC is to be connected manually
		if !proxyOnly {
			// exit early if spawning vnc client fails
			err := ln.SetDeadline(time.Now().Add(ListenTimeout))
			if err != nil {
				listenResChan <- err
			}
		}
		fd, err := ln.Accept()
		if err != nil {
			glog.V(2).Infof("Failed to accept unix sock connection. %s", err.Error())
			listenResChan <- err
		}
		defer fd.Close()

		glog.V(2).Infof("VNC Client connected in %v", time.Since(start))
		templates.PrintWarningForPausedVM(virtCli, vm, namespace)

		// write to FD <- pipeOutReader
		go func() {
			_, err := io.Copy(fd, pipeOutReader)
			readStop <- err
		}()

		// read from FD -> pipeInWriter
		go func() {
			_, err := io.Copy(pipeInWriter, fd)
			writeStop <- err
		}()

		// don't terminate until vnc client is done
		<-doneChan
		listenResChan <- err
	}()

	port := ln.Addr().(*net.TCPAddr).Port

	if proxyOnly {
		defer close(doneChan)
		optionString, err := json.Marshal(struct {
			Port int `json:"port"`
		}{port})
		if err != nil {
			return fmt.Errorf("error encountered: %s", err.Error())
		}
		fmt.Fprintln(cmd.OutOrStdout(), string(optionString))
	} else {
		// execute VNC Viewer
		go checkAndRunVNCViewer(doneChan, viewResChan, port)
	}

	go func() {
		defer close(stopChan)
		interrupt := make(chan os.Signal, 1)
		signal.Notify(interrupt, os.Interrupt)
		<-interrupt
	}()

	select {
	case <-stopChan:
	case err = <-readStop:
	case err = <-writeStop:
	case err = <-k8ResChan:
	case err = <-viewResChan:
	case err = <-listenResChan:
	}

	if err != nil {
		return fmt.Errorf("error encountered: %s", err.Error())
	}
	return nil
}

func checkAndRunVNCViewer(doneChan chan struct{}, viewResChan chan error, port int) {
	defer close(doneChan)
	var err error
	args := []string{}

	vncBin := ""
	osType := runtime.GOOS
	switch osType {
	case "darwin":
		if matches, err := filepath.Glob(MACOSTigerVNCPATTERN); err == nil && len(matches) > 0 {
			// Always use the latest version
			vncBin = matches[len(matches)-1]
			args = tigerVncArgs(port)
		} else if errors.Is(err, filepath.ErrBadPattern) {
			viewResChan <- err
			return
		} else if _, err := os.Stat(MACOSChickenVNC); err == nil {
			vncBin = MACOSChickenVNC
			args = chickenVncArgs(port)
		} else if !errors.Is(err, os.ErrNotExist) {
			viewResChan <- err
			return
		} else if _, err := os.Stat(MACOSRealVNC); err == nil {
			vncBin = MACOSRealVNC
			args = realVncArgs(port)
		} else if !errors.Is(err, os.ErrNotExist) {
			viewResChan <- err
			return
		} else if _, err := exec.LookPath(RemoteViewer); err == nil {
			// fall back to user supplied script/binary in path
			vncBin = RemoteViewer
			args = remoteViewerArgs(port)
		} else if !errors.Is(err, os.ErrNotExist) {
			viewResChan <- err
			return
		}
	case "linux", "windows":
		if _, err := exec.LookPath(RemoteViewer); err == nil {
			vncBin = RemoteViewer
			args = remoteViewerArgs(port)
		} else if _, err := exec.LookPath(TigerVNC); err == nil {
			vncBin = TigerVNC
			args = tigerVncArgs(port)
		} else {
			viewResChan <- fmt.Errorf("could not find %s or %s binary in $PATH",
				RemoteViewer, TigerVNC)
			viewResChan <- err
			return
		}
	default:
		viewResChan <- fmt.Errorf("virtctl does not support VNC on %v", osType)
		return
	}

	if vncBin == "" {
		glog.Errorf("No supported VNC app found in %s", osType)
		err = fmt.Errorf("No supported VNC app found in %s", osType)
	} else {
		if glog.V(4) {
			glog.Infof("Executing commandline: '%s %v'", vncBin, args)
		}
		// #nosec No risk for attacket injection. vncBin and args include predefined strings
		cmd := exec.Command(vncBin, args...)
		output, err := cmd.CombinedOutput()
		if err != nil {
			glog.Errorf("%s execution failed: %v, output: %v", vncBin, err, string(output))
		} else {
			glog.V(2).Infof("%v output: %v", vncBin, string(output))
		}
	}
	viewResChan <- err
}

func tigerVncArgs(port int) (args []string) {
	args = append(args, fmt.Sprintf(listenAddressFmt, port))
	if glog.V(4) {
		args = append(args, "Log=*:stderr:100")
	}
	return
}

func chickenVncArgs(port int) (args []string) {
	args = append(args, fmt.Sprintf(listenAddressFmt, port))
	return
}

func realVncArgs(port int) (args []string) {
	args = append(args, fmt.Sprintf(listenAddressFmt, port))
	args = append(args, "-WarnUnencrypted=0")
	args = append(args, "-Shared=0")
	args = append(args, "-ShareFiles=0")
	if glog.V(4) {
		args = append(args, "-log=*:stderr:100")
	}
	return
}

func remoteViewerArgs(port int) (args []string) {
	args = append(args, fmt.Sprintf("vnc://127.0.0.1:%d", port))
	if glog.V(4) {
		args = append(args, "--debug")
	}
	return
}

func usage() string {
	return `  # Connect to 'testvm' via remote-viewer:
   {{ProgramName}} vnc testvm`
}
