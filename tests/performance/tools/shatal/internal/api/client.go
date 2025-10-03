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

package api

import (
	"context"
	"encoding/base64"
	"fmt"
	"log/slog"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/kubectl/pkg/drain"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/shatal/internal/logger"
)

type Client struct {
	namespace      string
	resourcePrefix string
	crClient       client.Client
	clientset      *kubernetes.Clientset
	logger         *slog.Logger
}

func NewClient(kubeconfig, namespace, resourcePrefix string, log *slog.Logger) (*Client, error) {
	kubeconfigBase64, err := base64.StdEncoding.DecodeString(kubeconfig)
	if err != nil {
		return nil, err
	}

	clientConfig, err := clientcmd.NewClientConfigFromBytes(kubeconfigBase64)
	if err != nil {
		return nil, err
	}

	config, err := clientConfig.ClientConfig()
	if err != nil {
		return nil, err
	}

	scheme := runtime.NewScheme()
	err = v1alpha2.AddToScheme(scheme)
	if err != nil {
		return nil, err
	}

	err = corev1.AddToScheme(scheme)
	if err != nil {
		return nil, err
	}

	crClient, err := client.New(config, client.Options{
		Scheme: scheme,
	})
	if err != nil {
		return nil, err
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, err
	}

	return &Client{
		crClient:       crClient,
		clientset:      clientset,
		namespace:      namespace,
		resourcePrefix: resourcePrefix,
		logger:         log,
	}, nil
}

func (c *Client) DrainNode(ctx context.Context, node string) error {
	var evicted int
	var maxRetriesCount int
	retries := make(map[string]int)
	var mx sync.Mutex

	ctx, cancel := context.WithCancel(ctx)

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-time.After(5 * time.Second):
				c.logger.With("node", node).With("retries", maxRetriesCount).Info(fmt.Sprintf("Node draining... %d/%d", evicted, len(retries)))
			}
		}
	}()

	logWriter := logger.NewWriter(c.logger)

	err := drain.RunNodeDrain(&drain.Helper{
		Ctx:                 ctx,
		Client:              c.clientset,
		IgnoreAllDaemonSets: true,
		DeleteEmptyDirData:  true,
		PodSelector:         "vm=" + c.resourcePrefix,
		Out:                 logWriter,
		ErrOut:              logWriter,
		OnPodDeletionOrEvictionStarted: func(pod *corev1.Pod, usingEviction bool) {
			mx.Lock()
			defer mx.Unlock()

			count, ok := retries[pod.Name]
			if !ok {
				retries[pod.Name] = 1
				c.logger.With("eviction", usingEviction).Debug(fmt.Sprintf("Start pod eviction: %s", pod.Name))
				return
			}

			count++

			if count > maxRetriesCount {
				maxRetriesCount = count
			}

			retries[pod.Name]++
		},
		OnPodDeletionOrEvictionFinished: func(pod *corev1.Pod, usingEviction bool, err error) {
			if err != nil {
				c.logger.With("eviction", usingEviction).Error(fmt.Sprintf("Pod eviction failed: %s: %s", pod.Name, err))
				return
			}

			evicted++
			c.logger.With("eviction", usingEviction).Debug(fmt.Sprintf("Pod evicted: %s", pod.Name))
		},
	}, node)
	cancel()

	if err != nil {
		return err
	}

	c.logger.Info(fmt.Sprintf("Drained: %d/%d", evicted, len(retries)))

	return nil
}

func (c *Client) CordonNode(ctx context.Context, nodeName string) error {
	var node corev1.Node
	err := c.crClient.Get(ctx, types.NamespacedName{
		Name: nodeName,
	}, &node)
	if err != nil {
		return err
	}

	return drain.RunCordonOrUncordon(&drain.Helper{
		Ctx:    ctx,
		Client: c.clientset,
	}, &node, true)
}

func (c *Client) UnCordonNode(ctx context.Context, nodeName string) error {
	var node corev1.Node
	err := c.crClient.Get(ctx, types.NamespacedName{
		Name: nodeName,
	}, &node)
	if err != nil {
		return err
	}

	return drain.RunCordonOrUncordon(&drain.Helper{
		Ctx:    ctx,
		Client: c.clientset,
	}, &node, false)
}

func (c *Client) GetNodes(ctx context.Context, labelSelector string) ([]corev1.Node, error) {
	var options client.ListOptions
	if len(labelSelector) > 0 {
		var err error
		options.LabelSelector, err = labels.Parse(labelSelector)
		if err != nil {
			return nil, err
		}
	}

	var nodes corev1.NodeList
	err := c.crClient.List(ctx, &nodes, &options)
	if err != nil {
		return nil, err
	}

	return nodes.Items, nil
}

func (c *Client) GetVMs(ctx context.Context) ([]v1alpha2.VirtualMachine, error) {
	selector := labels.SelectorFromSet(map[string]string{"vm": c.resourcePrefix})

	var vms v1alpha2.VirtualMachineList
	err := c.crClient.List(ctx, &vms, &client.ListOptions{
		Namespace:     c.namespace,
		LabelSelector: selector,
	})
	if err != nil {
		return nil, err
	}

	return vms.Items, nil
}

func (c *Client) CreateVM(ctx context.Context, vm v1alpha2.VirtualMachine) error {
	return c.crClient.Create(ctx, &vm)
}

func (c *Client) DeleteVM(ctx context.Context, vm v1alpha2.VirtualMachine) error {
	return c.crClient.Delete(ctx, &vm)
}

func (c *Client) PatchCoreFraction(ctx context.Context, vm v1alpha2.VirtualMachine) error {
	restartApprovalModePatch := fmt.Sprintf(`{"spec":{"disraptions": {"restartApprovalMode": "%s"}}}`, vm.Spec.Disruptions.RestartApprovalMode)

	return c.crClient.Patch(ctx, &vm, client.RawPatch(types.MergePatchType, []byte(restartApprovalModePatch)))
}

func (c *Client) ApplyVMOP(ctx context.Context, vmop v1alpha2.VirtualMachineOperation) error {
	return c.crClient.Create(ctx, &vmop)
}
