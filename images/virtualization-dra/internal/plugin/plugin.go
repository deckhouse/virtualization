/*
Copyright 2025 Flant JSC

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

package plugin

import (
	"fmt"
	"os"
	"path"

	"github.com/spf13/pflag"
	"k8s.io/dynamic-resource-allocation/kubeletplugin"
)

var (
	KubeletPluginsDir  = kubeletplugin.KubeletPluginsDir
	KubeletRegistryDir = kubeletplugin.KubeletRegistryDir
)

const (
	pluginSocketFilename          = "dra.sock"
	registrarSocketFilenameSuffix = "-reg.sock"
)

func AddFlags(fs *pflag.FlagSet) {
	fs.StringVar(&KubeletPluginsDir, "kubelet-plugins-dir", KubeletPluginsDir, "Kubelet plugins directory")
	fs.StringVar(&KubeletRegistryDir, "kubelet-registry-dir", KubeletRegistryDir, "Kubelet registry directory")
}

func pluginDirPath(driverName string) string {
	return path.Join(KubeletPluginsDir, driverName)
}

func pluginSocketPath(driverName string) string {
	return path.Join(pluginDirPath(driverName), pluginSocketFilename)
}

func registrarDirPath() string {
	return KubeletRegistryDir
}

func registrarSocketFile(driverName string) string {
	return driverName + registrarSocketFilenameSuffix
}

func registrarSocketPath(driverName string) string {
	return path.Join(registrarDirPath(), registrarSocketFile(driverName))
}

func initPluginDir(driverName string) error {
	pluginDir := pluginDirPath(driverName)
	if err := os.MkdirAll(pluginDir, 0o700); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", pluginDir, err)
	}
	return nil
}
