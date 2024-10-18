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

package proxy

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/pprof"
	"net/url"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/tidwall/gjson"

	"github.com/deckhouse/kube-api-rewriter/pkg/kubevirt"
	"github.com/deckhouse/kube-api-rewriter/pkg/log"
	"github.com/deckhouse/kube-api-rewriter/pkg/rewriter"
	"github.com/deckhouse/kube-api-rewriter/pkg/server"
)

// PodJSON is a real Pod example to test JSON rewrites.
const PodJSON = `{
    "apiVersion": "v1",
    "kind": "Pod",
    "metadata": {
        "name": "example-pod",
        "annotations": {
            "cni.cilium.io/ipAddress": "10.66.10.1",
            "kubectl.kubernetes.io/default-container": "compute",
            "kubevirt.internal.virtualization.deckhouse.io/allow-pod-bridge-network-live-migration": "true",
            "kubevirt.internal.virtualization.deckhouse.io/domain": "cloud-alpine",
            "kubevirt.internal.virtualization.deckhouse.io/migrationTransportUnix": "true",
            "kubevirt.internal.virtualization.deckhouse.io/vm-generation": "1",
            "post.hook.backup.velero.io/command": "[\"/usr/bin/virt-freezer\", \"--unfreeze\", \"--name\", \"cloud-alpine\", \"--namespace\", \"vm\"]",
            "post.hook.backup.velero.io/container": "compute",
            "pre.hook.backup.velero.io/command": "[\"/usr/bin/virt-freezer\", \"--freeze\", \"--name\", \"cloud-alpine\", \"--namespace\", \"vm\"]",
            "pre.hook.backup.velero.io/container": "compute"
        },
        "creationTimestamp": "2024-10-01T11:45:59Z",
        "finalizers": [
            "virtualization.deckhouse.io/pod-protection"
        ],
        "generateName": "virt-launcher-cloud-alpine-",
        "labels": {
            "kubevirt.internal.virtualization.deckhouse.io": "virt-launcher",
            "kubevirt.internal.virtualization.deckhouse.io/created-by": "ac1e83d8-f2ad-4047-8ba9-3f557c687b9f",
            "kubevirt.internal.virtualization.deckhouse.io/nodeName": "virtlab-delivery-mi-2",
            "vm": "cloud-alpine",
            "vm-folder": "vm-cloud-alpine",
            "vm.kubevirt.internal.virtualization.deckhouse.io/name": "cloud-alpine"
        },
        "name": "virt-launcher-cloud-alpine-lxlz5",
        "namespace": "vm",
        "ownerReferences": [
            {
                "apiVersion": "internal.virtualization.deckhouse.io/v1",
                "blockOwnerDeletion": true,
                "controller": true,
                "kind": "InternalVirtualizationVirtualMachineInstance",
                "name": "cloud-alpine",
                "uid": "ac1e83d8-f2ad-4047-8ba9-3f557c687b9f"
            }
        ],
        "resourceVersion": "595346645",
        "uid": "68558c6e-aefb-4cbb-922a-e8389e8ce43f"
    },
    "spec": {
        "affinity": {
            "nodeAffinity": {
                "requiredDuringSchedulingIgnoredDuringExecution": {
                    "nodeSelectorTerms": [
                        {
                            "matchExpressions": [
                                {
                                    "key": "node-role.kubernetes.io/control-plane",
                                    "operator": "DoesNotExist"
                                }
                            ]
                        }
                    ]
                }
            }
        },
        "automountServiceAccountToken": false,
        "containers": [
            {
                "command": [
                    "/usr/bin/virt-launcher-monitor",
                    "--qemu-timeout",
                    "338s",
                    "--name",
                    "cloud-alpine",
                    "--uid",
                    "ac1e83d8-f2ad-4047-8ba9-3f557c687b9f",
                    "--namespace",
                    "vm",
                    "--kubevirt-share-dir",
                    "/var/run/kubevirt",
                    "--ephemeral-disk-dir",
                    "/var/run/kubevirt-ephemeral-disks",
                    "--container-disk-dir",
                    "/var/run/kubevirt/container-disks",
                    "--grace-period-seconds",
                    "75",
                    "--hook-sidecars",
                    "0",
                    "--ovmf-path",
                    "/usr/share/OVMF"
                ],
                "env": [
                    {
                        "name": "POD_NAME",
                        "valueFrom": {
                            "fieldRef": {
                                "apiVersion": "v1",
                                "fieldPath": "metadata.name"
                            }
                        }
                    }
                ],
                "image": "dev-registry.deckhouse.io/sys/deckhouse-oss/modules/virtualization@sha256:c3c6c6a87ce0082697da80a6e53b4bf59fb433be05cabd9f7c46201bd45283e6",
                "imagePullPolicy": "IfNotPresent",
                "name": "compute",
                "resources": {
                    "limits": {
                        "cpu": "4",
                        "devices.virtualization.deckhouse.io/kvm": "1",
                        "devices.virtualization.deckhouse.io/tun": "1",
                        "devices.virtualization.deckhouse.io/vhost-net": "1",
                        "memory": "4582277121"
                    },
                    "requests": {
                        "cpu": "4",
                        "devices.virtualization.deckhouse.io/kvm": "1",
                        "devices.virtualization.deckhouse.io/tun": "1",
                        "devices.virtualization.deckhouse.io/vhost-net": "1",
                        "ephemeral-storage": "50M",
                        "memory": "4582277121"
                    }
                },
                "securityContext": {
                    "capabilities": {
                        "add": [
                            "NET_BIND_SERVICE",
                            "SYS_NICE"
                        ]
                    },
                    "privileged": false,
                    "runAsNonRoot": false,
                    "runAsUser": 0
                },
                "terminationMessagePath": "/dev/termination-log",
                "terminationMessagePolicy": "File",
                "volumeDevices": [
                    {
                        "devicePath": "/dev/vd-cloud-alpine",
                        "name": "vd-cloud-alpine"
                    },
                    {
                        "devicePath": "/dev/vd-cloud-alpine-data",
                        "name": "vd-cloud-alpine-data"
                    }
                ],
                "volumeMounts": [
                    {
                        "mountPath": "/var/run/kubevirt-private",
                        "name": "private"
                    },
                    {
                        "mountPath": "/var/run/kubevirt",
                        "name": "public"
                    },
                    {
                        "mountPath": "/var/run/kubevirt-ephemeral-disks",
                        "name": "ephemeral-disks"
                    },
                    {
                        "mountPath": "/var/run/kubevirt/container-disks",
                        "mountPropagation": "HostToContainer",
                        "name": "container-disks"
                    },
                    {
                        "mountPath": "/var/run/libvirt",
                        "name": "libvirt-runtime"
                    },
                    {
                        "mountPath": "/var/run/kubevirt/sockets",
                        "name": "sockets"
                    },
                    {
                        "mountPath": "/var/run/kubevirt/hotplug-disks",
                        "mountPropagation": "HostToContainer",
                        "name": "hotplug-disks"
                    }
                ]
            }
        ],

        "dnsPolicy": "ClusterFirst",
        "enableServiceLinks": false,
        "hostname": "cloud-alpine",
        "nodeName": "virtlab-delivery-mi-2",
        "nodeSelector": {
            "cpu-model.node.virtualization.deckhouse.io/Nehalem": "true",
            "kubernetes.io/arch": "amd64",
            "kubevirt.internal.virtualization.deckhouse.io/schedulable": "true"
        },
        "preemptionPolicy": "PreemptLowerPriority",
        "priority": 1000,
        "priorityClassName": "develop",
        "readinessGates": [
            {
                "conditionType": "kubevirt.io/virtual-machine-unpaused"
            }
        ],
        "restartPolicy": "Never",
        "schedulerName": "linstor",
        "securityContext": {
            "runAsUser": 0
        },
        "serviceAccount": "default",
        "serviceAccountName": "default",
        "terminationGracePeriodSeconds": 90,

        "tolerations": [
            {
                "effect": "NoExecute",
                "key": "node.kubernetes.io/not-ready",
                "operator": "Exists",
                "tolerationSeconds": 300
            },
            {
                "effect": "NoExecute",
                "key": "node.kubernetes.io/unreachable",
                "operator": "Exists",
                "tolerationSeconds": 300
            },
            {
                "effect": "NoSchedule",
                "key": "node.kubernetes.io/memory-pressure",
                "operator": "Exists"
            },
            {
                "effect": "NoSchedule",
                "key": "devices.virtualization.deckhouse.io/kvm",
                "operator": "Exists"
            },
            {
                "effect": "NoSchedule",
                "key": "devices.virtualization.deckhouse.io/tun",
                "operator": "Exists"
            },
            {
                "effect": "NoSchedule",
                "key": "devices.virtualization.deckhouse.io/vhost-net",
                "operator": "Exists"
            }
        ],

        "volumes": [
            {
                "emptyDir": {},
                "name": "private"
            },
            {
                "emptyDir": {},
                "name": "public"
            },
            {
                "emptyDir": {},
                "name": "sockets"
            },
            {
                "emptyDir": {},
                "name": "virt-bin-share-dir"
            },
            {
                "emptyDir": {},
                "name": "libvirt-runtime"
            },
            {
                "emptyDir": {},
                "name": "ephemeral-disks"
            },
            {
                "emptyDir": {},
                "name": "container-disks"
            },
            {
                "name": "vd-cloud-alpine",
                "persistentVolumeClaim": {
                    "claimName": "vd-cloud-alpine-30e0ce5d-d0d7-4f38-b0a2-493330e5bb4a"
                }
            },
            {
                "name": "vd-cloud-alpine-data",
                "persistentVolumeClaim": {
                    "claimName": "vd-cloud-alpine-data-23941f64-7241-40a1-8fc1-f976c7c364e8"
                }
            },
            {
                "emptyDir": {},
                "name": "hotplug-disks"
            }
        ]
    },
	"status": {
        "conditions": [
            {
                "lastProbeTime": null,
                "lastTransitionTime": null,
                "status": "False",
                "type": "Custom"
            },
            {
                "lastProbeTime": "2024-10-01T11:45:59Z",
                "lastTransitionTime": "2024-10-01T11:45:59Z",
                "message": "the virtual machine is not paused",
                "reason": "NotPaused",
                "status": "True",
                "type": "kubevirt.io/virtual-machine-unpaused"
            },
            {
                "lastProbeTime": null,
                "lastTransitionTime": "2024-10-01T11:45:59Z",
                "status": "True",
                "type": "Initialized"
            },
            {
                "lastProbeTime": null,
                "lastTransitionTime": "2024-10-01T11:46:01Z",
                "status": "True",
                "type": "Ready"
            },
            {
                "lastProbeTime": null,
                "lastTransitionTime": "2024-10-01T11:46:01Z",
                "status": "True",
                "type": "ContainersReady"
            },
            {
                "lastProbeTime": null,
                "lastTransitionTime": "2024-10-01T11:45:59Z",
                "status": "True",
                "type": "PodScheduled"
            }
        ],
        "containerStatuses": [
            {
                "containerID": "containerd://4305d5ef79c16cbb9f28450506f9ec4650269e8034bdd0d5d42189aa638effb4",
                "image": "sha256:cf321ffda57daa4fbf19daf047506fd36a841ced39ef869a80cc53a6387bba26",
                "imageID": "dev-registry.deckhouse.io/sys/deckhouse-oss/modules/virtualization@sha256:c3c6c6a87ce0082697da80a6e53b4bf59fb433be05cabd9f7c46201bd45283e6",
                "lastState": {},
                "name": "compute",
                "ready": true,
                "restartCount": 0,
                "started": true,
                "state": {
                    "running": {
                        "startedAt": "2024-10-01T11:46:01Z"
                    }
                }
            }
        ],
        "hostIP": "172.18.18.72",
        "phase": "Running",
        "podIP": "10.66.10.1",
        "podIPs": [
            {
                "ip": "10.66.10.1"
            }
        ],
        "qosClass": "Guaranteed",
        "startTime": "2024-10-01T11:45:59Z"
    }
}`

// Test_run_proxy_with_pprof runs server, rewriter and a client
// in different go routines for experimenting with pprof.
//
// Start test and run go tool:
//
//	go tool pprof -http=127.0.0.1:8085 http://127.0.0.1:43200/debug/pprof/heap
func Test_run_proxy_with_pprof(t *testing.T) {
	// Comment to run experiments.
	t.SkipNow()

	// Memory stats printer.
	go func() {
		ticker := time.NewTicker(3 * time.Second)
		for {
			<-ticker.C
			var stats runtime.MemStats
			runtime.ReadMemStats(&stats)
			fmt.Printf(
				"Heap Alloc: %0.2f MB, Heap InUse %0.2f MB\n",
				float64(stats.HeapAlloc)/1024/1024,
				float64(stats.HeapInuse)/1024/1024,
			)
		}
	}()

	// Pprof server
	go func() {
		mux := http.NewServeMux()

		mux.HandleFunc("/debug/pprof/", pprof.Index)
		mux.HandleFunc("/debug/pprof/cmdline", pprof.Cmdline)
		mux.HandleFunc("/debug/pprof/profile", pprof.Profile)
		mux.HandleFunc("/debug/pprof/symbol", pprof.Symbol)
		mux.HandleFunc("/debug/pprof/trace", pprof.Trace)

		pprofSrv := &http.Server{
			Addr:    "127.0.0.1:43200",
			Handler: mux,
		}

		fmt.Println("Pprof server started at 127.0.0.1:43200")
		if err := pprofSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fmt.Println("Error starting pprof server:", err)
		}
	}()

	// This HTTP server implements List Pods endpoint of the Kubernetes API Server.
	kubeAPIRready := make(chan struct{}, 0)
	// Change count to stress test the rewriter.
	podsCount := 3200
	go func() {
		items := strings.Repeat(PodJSON+",", podsCount-1)
		PodsListJSON := `{"apiVersion":"v1", "kind":"PodList", "items":[` + items + PodJSON + `]}`

		once := 0

		handleGet := func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Content-Length", strconv.Itoa(len(PodsListJSON)))
			w.WriteHeader(http.StatusOK)
			wrbytes, err := io.Copy(w, bytes.NewBuffer([]byte(PodsListJSON)))
			if err != nil {
				t.Fatalf("Should send pod list: %v", err)
			}

			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}

			if once == 0 {
				fmt.Printf("pods requested, send %d bytes (%d written)\n", len(PodsListJSON), wrbytes)
				once = 1
			}
		}

		handleRequest := func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodGet:
				handleGet(w, r)
			default:
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			}
		}

		mux := http.NewServeMux()
		mux.HandleFunc("/api/v1/namespaces/vm/pods", handleRequest)

		kubeAPISrv := &http.Server{
			Addr:    "127.0.0.1:43215",
			Handler: mux,
		}

		fmt.Println("Server started at 127.0.0.1:43215")
		close(kubeAPIRready)
		if err := kubeAPISrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fmt.Println("Error starting server:", err)
		}
	}()

	// This HTTP server runs the rewriter. Switch client to use it to detect problems with proxy handler.
	go func() {
		items := strings.Repeat(PodJSON+",", podsCount-1)
		PodsListJSON := `{"apiVersion":"v1", "kind":"PodList", "items":[` + items + PodJSON + `]}`

		rewriteRules := kubevirt.KubevirtRewriteRules
		rewriteRules.Init()

		rwr := &rewriter.RuleBasedRewriter{
			Rules: rewriteRules,
		}

		once := 0

		handleGet := func(w http.ResponseWriter, r *http.Request) {
			rwrBytes, err := rwr.RewriteJSONPayload(nil, []byte(PodsListJSON), rewriter.Rename)
			if err != nil {
				t.Fatalf("Should rewrite JSON pod list: %v", err)
				return
			}

			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Content-Length", strconv.Itoa(len(rwrBytes)))
			w.WriteHeader(http.StatusOK)
			wrbytes, err := io.Copy(w, bytes.NewBuffer(rwrBytes))
			if err != nil {
				t.Fatalf("Should send pod list: %v", err)
			}

			if flusher, ok := w.(http.Flusher); ok {
				flusher.Flush()
			}

			if once == 0 {
				fmt.Printf("pods requested, send %d bytes (%d written)\n", len(PodsListJSON), wrbytes)
				once = 1
			}
		}

		handleRequest := func(w http.ResponseWriter, r *http.Request) {
			switch r.Method {
			case http.MethodGet:
				handleGet(w, r)
			default:
				http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			}
		}

		mux := http.NewServeMux()
		mux.HandleFunc("/api/v1/namespaces/vm/pods", handleRequest)

		kubeAPISrv := &http.Server{
			Addr:    "127.0.0.1:43217",
			Handler: mux,
		}

		fmt.Println("Server started at 127.0.0.1:43217")
		if err := kubeAPISrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fmt.Println("Error starting server:", err)
		}
	}()

	// A rewriter proxy.
	go func() {
		log.SetupDefaultLoggerFromEnv(log.Options{
			Level:  "debug",
			Format: "pretty",
			Output: "discard",
		})
		//slog.SetDefault(slog.New(slog.NewTextHandler(io.Discard, nil)))

		apiServerURL := "http://127.0.0.1:43215"
		targetURL, err := url.Parse(apiServerURL)
		if err != nil {
			t.Fatalf("Should parse url %s: %v", apiServerURL, err)
			return
		}

		rewriteRules := kubevirt.KubevirtRewriteRules
		rewriteRules.Init()

		rwr := &rewriter.RuleBasedRewriter{
			Rules: rewriteRules,
		}
		proxyHandler := &Handler{
			Name:         "test-mem-leak",
			TargetClient: &http.Client{},
			TargetURL:    targetURL,
			ProxyMode:    ToRenamed,
			Rewriter:     rwr,
		}

		srv := &server.HTTPServer{
			InstanceDesc: "Test Mem Leak",
			ListenAddr:   "127.0.0.1:43216",
			RootHandler:  proxyHandler,
		}

		srv.Start()
	}()

	<-kubeAPIRready

	fmt.Println("Start spamming ...")

	// Spam proxy with requests.
	start := time.Now()
	spamDuration := time.Minute
	sleepDuration := time.Minute
	//maxCount := 2200000
	count := 1
	for {
		// Choose what source to test.
		// No proxy, no rewrites.
		// req, err := http.NewRequest("GET", "http://127.0.0.1:43215/api/v1/namespaces/vm/pods", nil)
		// No proxy, only rewriter.
		req, err := http.NewRequest("GET", "http://127.0.0.1:43217/api/v1/namespaces/vm/pods", nil)
		// Proxy and rewriter.
		// req, err := http.NewRequest("GET", "http://127.0.0.1:43216/api/v1/namespaces/vm/pods", nil)
		if err != nil {
			t.Fatalf("Should not fail on creating request %d: %v", count, err)
			return
		}

		startRequest := time.Now()

		resp, err := http.DefaultClient.Do(req)

		if err != nil {
			t.Fatalf("Should not fail on GET request %d: %v", count, err)
			return
		}

		startRead := time.Now()

		podBytes, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatalf("Should not fail on reading response %d: %v", count, err)
			return
		}
		endRead := time.Now()

		resp.Body.Close()

		respKind := gjson.GetBytes(podBytes, "kind").String()
		if respKind != "PodList" {
			t.Fatalf("Got unexpected kind: %s", respKind)
			return
		}

		endKind := time.Now()

		if count == 1 {
			dur := endRead.Sub(startRequest)
			speed := float64(len(podBytes)) / dur.Seconds() / 1024
			fmt.Printf("Request time: %s, Speed: %0.2f kb/s\n", startRead.Sub(startRequest).Truncate(time.Millisecond).String(), speed)
			fmt.Printf("Read time: %s, Speed: %0.2f kb/s\n", endRead.Sub(startRead).Truncate(time.Millisecond).String(), speed)
			fmt.Printf("Whole time: %s\n", endKind.Sub(startRequest).Truncate(time.Millisecond).String())
			fmt.Printf("%d. Got %s. Read %d bytes.\n", count, respKind, len(podBytes))
		}

		now := time.Now()
		if now.Sub(start) > spamDuration {
			fmt.Printf("Send %d requests in %s\n", count, now.Sub(start).Truncate(time.Second).String())
			break
		}

		podBytes = nil

		count++
		//if count == maxCount {
		//	return
		//}
	}

	time.Sleep(sleepDuration)
}

// Test_RewriteJSONPayload_time runs RewriteJSONPayload
// with different PodList lengths and outputs time stats.
//
// Example:
//
//	=== RUN   Test_RewriteJSONPayload_time
//	Got 9 results
//	  100  expect:      1.78s  got:      1.78s  x1.00
//	  200  expect:      3.56s  got:     1.875s  x0.53
//	  400  expect:      7.12s  got:      2.39s  x0.34
//	  800  expect:     14.24s  got:      3.83s  x0.27
//	 1600  expect:     28.48s  got:     4.709s  x0.17
//	 3200  expect:     56.96s  got:     6.077s  x0.11
//	 6400  expect:  1m53.921s  got:     8.396s  x0.07
//	12800  expect:  3m47.842s  got:    12.013s  x0.05
//	25600  expect:  7m35.685s  got:    17.271s  x0.04
func Test_RewriteJSONPayload_time(t *testing.T) {
	t.SkipNow()

	rewriteRules := kubevirt.KubevirtRewriteRules
	rewriteRules.Init()

	rwr := &rewriter.RuleBasedRewriter{
		Rules: rewriteRules,
	}

	podListCounts := []int{
		100,
		200,
		400,
		800,
		1600,
		3200,
		6400,
		12800,
		25600,
	}

	var wg sync.WaitGroup
	wg.Add(len(podListCounts))

	type testRes struct {
		count         int
		execDur       time.Duration
		bytesCount    int
		rwrBytesCount int
	}

	resCh := make(chan testRes, len(podListCounts))

	for _, podListCount := range podListCounts {
		go func(podsCount int) {
			// Construct PodList with podsCount items. Name uniqueness
			// is not significant for the test purposes.
			items := strings.Repeat(PodJSON+",", podsCount-1)
			podsListJSON := `{"apiVersion":"v1", "kind":"PodList", "items":[` + items + PodJSON + `]}`

			start := time.Now()
			rwrBytes, err := rwr.RewriteJSONPayload(nil, []byte(podsListJSON), rewriter.Restore)
			if err != nil {
				t.Fatalf("Should rewrite JSON: %v", err)
				return
			}
			end := time.Now()

			resCh <- testRes{
				count:         podsCount,
				execDur:       end.Sub(start),
				bytesCount:    len(podsListJSON),
				rwrBytesCount: len(rwrBytes),
			}

			wg.Done()
		}(podListCount)
	}

	wg.Wait()

	// Extract results from the chan.
	testResults := make([]testRes, 0, len(podListCounts))
	for range podListCounts {
		res := <-resCh
		testResults = append(testResults, res)
	}

	// Print sorted results.
	fmt.Printf("Got %d results\n", len(testResults))
	sort.SliceStable(testResults, func(i, j int) bool {
		return testResults[i].count < testResults[j].count
	})
	first := testResults[0]
	for _, res := range testResults {
		expectedDur := time.Duration(res.count/first.count) * first.execDur
		ratio := float64(res.execDur) / float64(expectedDur)

		fmt.Printf("%5d  expect: %10s  got: %10s  x%0.2f\n",
			res.count,
			expectedDur.Truncate(time.Millisecond).String(),
			res.execDur.Truncate(time.Millisecond).String(),
			ratio,
		)
	}
}
