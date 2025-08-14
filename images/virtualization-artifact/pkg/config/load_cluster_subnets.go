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

package config

import (
	"fmt"
	"net/netip"
	"os"
)

const (
	// ClusterPodSubnetVar is an env variable holds global podSubnetCIDR value.
	ClusterPodSubnetVar = "CLUSTER_POD_SUBNET_CIDR"
	// ClusterServiceSubnetVar is an env variable holds global serviceSubnetCIDR value.
	ClusterServiceSubnetVar = "CLUSTER_SERVICE_SUBNET_CIDR"
)

type ClusterSubnets struct {
	PodSubnet     netip.Prefix
	ServiceSubnet netip.Prefix
}

func LoadClusterSubnetsFromEnvs() (*ClusterSubnets, error) {
	podSubnetStr := os.Getenv(ClusterPodSubnetVar)
	if podSubnetStr == "" {
		return nil, fmt.Errorf("environment variable %q undefined, specify global podSubnetCIDR from cluster configuration", ClusterPodSubnetVar)
	}

	podSubnet, err := netip.ParsePrefix(podSubnetStr)
	if err != nil {
		return nil, fmt.Errorf("parse podSubnetCIDR: %w", err)
	}

	serviceSubnetStr := os.Getenv(ClusterServiceSubnetVar)
	if serviceSubnetStr == "" {
		return nil, fmt.Errorf("environment variable %q undefined, specify global serviceSubnetCIDR from cluster configuration", ClusterServiceSubnetVar)
	}

	serviceSubnet, err := netip.ParsePrefix(serviceSubnetStr)
	if err != nil {
		return nil, fmt.Errorf("parse podSubnetCIDR: %w", err)
	}

	return &ClusterSubnets{
		PodSubnet:     podSubnet,
		ServiceSubnet: serviceSubnet,
	}, nil
}
