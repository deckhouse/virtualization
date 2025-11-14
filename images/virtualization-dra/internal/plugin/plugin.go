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

	"k8s.io/dynamic-resource-allocation/kubeletplugin"
)

var (
	kubeletPluginsDir  = kubeletplugin.KubeletPluginsDir
	kubeletRegistryDir = kubeletplugin.KubeletRegistryDir
)

const (
	virtualizationPluginSocketFilename    = "dra.sock"
	virtualizationRegistrarSocketFilename = driverName + "-reg.sock"
)

func virtualizationPluginDirPath() string {
	return path.Join(kubeletPluginsDir, driverName)
}

func virtualizationPluginSocketPath() string {
	return path.Join(virtualizationPluginDirPath(), virtualizationPluginSocketFilename)
}

func virtualizationRegistrarDirPath() string {
	return kubeletRegistryDir
}

func virtualizationRegistrarSocketPath() string {
	return path.Join(virtualizationRegistrarDirPath(), virtualizationRegistrarSocketFilename)
}

func InitPluginDirs(setKubeletPluginsDir, setKubeletRegistryDir string) error {
	if setKubeletPluginsDir != "" {
		kubeletPluginsDir = setKubeletPluginsDir
	}

	pluginDir := virtualizationPluginDirPath()
	if err := os.MkdirAll(pluginDir, 0700); err != nil {
		return fmt.Errorf("failed to create directory %s: %w", pluginDir, err)
	}

	if setKubeletRegistryDir != "" {
		kubeletRegistryDir = setKubeletRegistryDir
	}

	return nil
}
