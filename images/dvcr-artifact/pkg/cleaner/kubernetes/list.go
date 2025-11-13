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

package kubernetes

import (
	"context"
	"fmt"

	"github.com/spf13/pflag"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"

	"github.com/deckhouse/virtualization/api/client/kubeclient"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type Client struct {
	virtClient kubeclient.Client
	kubeClient *kubernetes.Clientset
}

func NewVirtualizationClient() (*Client, error) {
	clientConfig := kubeclient.DefaultClientConfig(&pflag.FlagSet{})
	client, err := kubeclient.GetClientFromClientConfig(clientConfig)
	if err != nil {
		return nil, fmt.Errorf("init client for virtualization API: %w", err)
	}

	restConfig, err := clientConfig.ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("init rest config for kubernetes API: %w", err)
	}
	kubeClient, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("init client for kubernetes API: %w", err)
	}

	return &Client{
		kubeClient: kubeClient,
		virtClient: client,
	}, nil
}

type ImageInfo struct {
	Type      string
	Namespace string
	Name      string
	Phase     v1alpha2.DiskPhase
}

func (c *Client) ListAllPossibleImages(ctx context.Context) ([]ImageInfo, error) {
	clusterVirtualImages, err := c.ListClusterVirtualImages(ctx)
	if err != nil {
		return nil, err
	}

	virtualImages, err := c.ListVirtualImagesAll(ctx)
	if err != nil {
		return nil, err
	}

	virtualDisks, err := c.ListVirtualDisksAll(ctx)
	if err != nil {
		return nil, err
	}

	fmt.Printf("Found %d cvi, %d vi, %d vd resources in cluster\n",
		len(clusterVirtualImages),
		len(virtualImages),
		len(virtualDisks),
	)
	// Return all 3 arrays.
	clusterVirtualImages = append(clusterVirtualImages, virtualImages...)
	return append(clusterVirtualImages, virtualDisks...), nil
}

func (c *Client) ListClusterVirtualImages(ctx context.Context) ([]ImageInfo, error) {
	resources, err := c.virtClient.ClusterVirtualImages().List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	images := make([]ImageInfo, 0, len(resources.Items))
	for _, resource := range resources.Items {
		image := ImageInfo{
			Type: v1alpha2.ClusterVirtualImageKind,
			Name: resource.GetName(),
		}
		images = append(images, image)
	}
	return images, nil
}

func (c *Client) ListVirtualImagesAll(ctx context.Context) ([]ImageInfo, error) {
	resources, err := c.virtClient.VirtualImages("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	images := make([]ImageInfo, 0, len(resources.Items))
	for _, resource := range resources.Items {
		image := ImageInfo{
			Type:      v1alpha2.VirtualImageKind,
			Namespace: resource.GetNamespace(),
			Name:      resource.GetName(),
		}
		images = append(images, image)
	}
	return images, nil
}

func (c *Client) ListVirtualDisksAll(ctx context.Context) ([]ImageInfo, error) {
	resources, err := c.virtClient.VirtualDisks("").List(ctx, metav1.ListOptions{})
	if err != nil {
		return nil, err
	}

	images := make([]ImageInfo, 0, len(resources.Items))
	for _, resource := range resources.Items {
		image := ImageInfo{
			Type:      v1alpha2.VirtualDiskKind,
			Namespace: resource.GetNamespace(),
			Name:      resource.GetName(),
			Phase:     resource.Status.Phase,
		}
		images = append(images, image)
	}
	return images, nil
}

const (
	maintenanceSecretNS   = "d8-virtualization"
	maintenanceSecretName = "dvcr-maintenance"
)

func (c *Client) GetMaintenanceSecret(ctx context.Context) (*corev1.Secret, error) {
	secret, err := c.kubeClient.CoreV1().Secrets(maintenanceSecretNS).Get(ctx, maintenanceSecretName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("get maintenance secret: %w", err)
	}

	return secret, nil
}

func (c *Client) UpdateMaintenanceSecret(ctx context.Context, secret *corev1.Secret) error {
	_, err := c.kubeClient.CoreV1().Secrets(maintenanceSecretNS).
		Update(ctx, secret, metav1.UpdateOptions{})

	if err != nil {
		return fmt.Errorf("annotate maintenance secret: %w", err)
	}

	return nil
}
