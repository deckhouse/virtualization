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
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/ilyakaznacheev/cleanenv"
	_ "github.com/joho/godotenv/autoload"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

type Resource struct {
	GVR       schema.GroupVersionResource `json:"gvr"`
	Name      string                      `json:"name"`
	Namespace string                      `json:"namespace,omitempty"`
}

func (r *Resource) GetGVR() string {
	return fmt.Sprintf("%s %s/%s", r.GVR.Resource, r.GVR.Group, r.GVR.Version)
}

func logInfo(msg string, r *Resource) {
	slog.Info(msg, slog.String("gvr", r.GetGVR()), slog.String("namespace", r.Namespace), slog.String("name", r.Name))
}

func logError(msg string, err error, r *Resource) {
	slog.Error(msg, slog.String("gvr", r.GetGVR()), slog.String("namespace", r.Namespace), slog.String("name", r.Name), slog.Any("err", err))
}

type PreDeleteHook struct {
	dynamicClient   dynamic.Interface
	resources       []Resource
	KubeConfigPath  string        `env:"KUBECONFIG"`
	ResourcesString string        `env:"RESOURCES"`
	WaitTimeOut     time.Duration `env:"WAIT_TIMEOUT" env-default:"300s"`
}

func NewPreDeleteHook() (*PreDeleteHook, error) {
	var resources []Resource
	var clusterConfig *rest.Config
	var err error
	p := &PreDeleteHook{}

	err = cleanenv.ReadEnv(p)
	if err != nil {
		return nil, fmt.Errorf("can't load env:  %v", err)
	}

	if p.ResourcesString == "" {
		return nil, fmt.Errorf("RESOURCES env can't be empty")
	}
	err = json.Unmarshal([]byte(p.ResourcesString), &resources)
	if err != nil {
		return nil, fmt.Errorf("can't parse RESOURCES env: %v", err)
	}

	if p.KubeConfigPath != "" {
		clusterConfig, err = clientcmd.BuildConfigFromFlags("", p.KubeConfigPath)
	} else {
		clusterConfig, err = rest.InClusterConfig()
	}
	if err != nil {
		return nil, fmt.Errorf("can't create k8s config: %v", err)
	}

	dynamicClient, err := dynamic.NewForConfig(clusterConfig)
	if err != nil {
		return nil, fmt.Errorf("can't create dynamic client: %v", err)
	}

	p.dynamicClient = dynamicClient
	p.resources = resources
	return p, nil
}

// Attempts to delete a resource object with a backoff strategy in case of failures.
// Retries deletion for a maximum number of attempts with increasing intervals.
func (p *PreDeleteHook) deleteResourceBackOff(r *Resource) error {

	var (
		err           error
		maxRetries    = 5
		backOffFactor = 2
		retryInterval = 1 * time.Second
	)

	for i := 0; i < maxRetries; i++ {
		err = p.dynamicClient.Resource(r.GVR).Namespace(r.Namespace).Delete(context.TODO(), r.Name, metav1.DeleteOptions{})

		if err == nil || errors.IsNotFound(err) {
			break
		}
		time.Sleep(retryInterval)
		retryInterval = retryInterval * time.Duration(backOffFactor)
		logError("Failed to delete resource, trying again", err, r)
		continue
	}
	return err
}

// Waits for the specified resource to be deleted within a given timeout period.
// Continuously checks the existence of the resource until the resource is not found
// or the timeout period is reached.
func (p *PreDeleteHook) waitForResourceToBeDeleted(r *Resource) error {
	var err error
	deadline := time.Now().Add(p.WaitTimeOut)
	for time.Now().Before(deadline) {
		_, err = p.dynamicClient.Resource(r.GVR).Namespace(r.Namespace).Get(context.TODO(), r.Name, metav1.GetOptions{})
		if errors.IsNotFound(err) {
			return nil
		}
		if err != nil {
			logError("Failed to check resource, trying again", err, r)
			continue
		}
		logInfo("Waiting resource to be deleted", r)
		time.Sleep(2 * time.Second)
	}
	return err
}

func (p *PreDeleteHook) Run() {
	var wg sync.WaitGroup
	for _, resource := range p.resources {
		wg.Add(1)
		go func(r *Resource) {
			defer wg.Done()
			var err error
			logInfo("Trying to delete...", r)

			if err = p.deleteResourceBackOff(r); err != nil {
				logError("Failed to delete resource", err, r)
				return
			}

			if err = p.waitForResourceToBeDeleted(r); err != nil {
				logError("Failed to check resource", err, r)
				return
			}
			logInfo("Resource deleted", r)
		}(&resource)
	}
	wg.Wait()
}

func main() {
	hook, err := NewPreDeleteHook()
	if err != nil {
		slog.Error("Can't create PreDeleteHook", slog.Any("err", err))
		os.Exit(0)
	}
	hook.Run()
}
