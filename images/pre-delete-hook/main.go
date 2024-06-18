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
	"sync"
	"time"

	"github.com/ilyakaznacheev/cleanenv"
	_ "github.com/joho/godotenv/autoload"
	log "github.com/sirupsen/logrus"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

const WaitTimeout = 30 * time.Second

type Resource struct {
	GVR       schema.GroupVersionResource `json:"gvr"`
	Name      string                      `json:"name"`
	Namespace string                      `json:"namespace,omitempty"`
}

func (r *Resource) ShowName() string {
	if r.Namespace == "" {
		return fmt.Sprintf("%s %s/%s %s", r.GVR.Resource, r.GVR.Group, r.GVR.Version, r.Name)
	}
	return fmt.Sprintf("%s %s/%s %s/%s", r.GVR.Resource, r.GVR.Group, r.GVR.Version, r.Namespace, r.Name)
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

func (p *PreDeleteHook) Run() {
	var wg sync.WaitGroup
	for _, resource := range p.resources {

		log.Infof("Deleting resource %s ...", resource.ShowName())
		wg.Add(1)

		go func(r *Resource) {
			defer wg.Done()
			err := p.dynamicClient.Resource(r.GVR).Namespace(r.Namespace).Delete(context.TODO(), r.Name, metav1.DeleteOptions{})

			if err != nil {
				if errors.IsNotFound(err) {
					log.Warnf("Resource %s not found", r.ShowName())
					return
				}
				log.Errorf("Can't' delete resource %s: %v", r.ShowName(), err)
				return
			}

			deadline := time.Now().Add(p.WaitTimeOut)
			for time.Now().Before(deadline) {
				_, err := p.dynamicClient.Resource(r.GVR).Namespace(r.Namespace).Get(context.TODO(), r.Name, metav1.GetOptions{})
				if errors.IsNotFound(err) {
					log.Infof("Resource %s is deleted...", r.ShowName())
					return
				}
				if err != nil {
					log.Errorf("Failed to check resource %s: %v", r.ShowName(), err)
					return
				}

				log.Infof("Waiting for resource %s to be deleted...", r.ShowName())
				time.Sleep(2 * time.Second)
			}
		}(&resource)

	}

	wg.Wait()
}

func main() {
	hook, err := NewPreDeleteHook()
	if err != nil {
		log.Fatalf("Can't create PreDeleteHook:  %v", err)
	}
	hook.Run()
}
