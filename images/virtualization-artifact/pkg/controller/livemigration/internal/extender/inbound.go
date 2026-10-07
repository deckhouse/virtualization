/*
Copyright 2026 Flant JSC

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

package extender

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/deckhouse/pkg/log"
	"github.com/deckhouse/virtualization-controller/pkg/controller/indexer"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
)

// kube-scheduler reads pods straight from the apiserver, so the pod in a request carries the
// labels as kube-api-rewriter stores them, not the kubevirt.io keys the controller cache shows.
const (
	launcherAppLabel  = "kubevirt.internal.virtualization.deckhouse.io"
	migrationJobLabel = "kubevirt.internal.virtualization.deckhouse.io/migrationJobUID"
	launcherAppName   = "virt-launcher"
)

// InboundMigrationExtender scores the candidate nodes of a live migration target: a node no other
// migration target heads to gets a small bonus, so targets spread over the nodes with free
// incoming slots while the resources of the candidates are close. It never filters nodes out,
// so a virtual machine bound to a few nodes by its devices or affinity always gets a target; the
// migration slot limits hold when a migration starts transferring memory.
type InboundMigrationExtender struct {
	client client.Reader
	// countOutgoing adds the node's outgoing migrations that transfer memory to its load: with a
	// shared per-node budget they take the same slots as the incoming ones.
	countOutgoing bool
	log           *log.Logger
}

// Settings configures the extender.
type Settings struct {
	// CountOutgoing counts the outgoing migrations transferring memory as the node's load.
	CountOutgoing bool
}

// freeNodeScore is the score of a node without migration load. With the extender weight of 1,
// kube-scheduler turns it into 10 points against up to 200 of the resource scoring, so the slots
// only decide between nodes whose free resources differ by less than about a tenth.
const freeNodeScore int64 = 1

func NewInboundMigrationExtender(client client.Reader, settings Settings, log *log.Logger) *InboundMigrationExtender {
	return &InboundMigrationExtender{
		client:        client,
		countOutgoing: settings.CountOutgoing,
		log:           log,
	}
}

// Filter keeps every candidate. Deckhouse before 1.75 cannot switch the filter verb off and calls
// it for every pod, so the extender answers it without filtering anything out.
func (e *InboundMigrationExtender) Filter(_ context.Context, args Args) FilterResult {
	if args.NodeNames == nil {
		// An empty result would filter every node out; an error is skipped by the scheduler.
		return FilterResult{Error: "node names are expected: the extender must be registered with nodeCacheCapable"}
	}
	return FilterResult{NodeNames: args.NodeNames}
}

func (e *InboundMigrationExtender) Prioritize(ctx context.Context, args Args) []HostPriority {
	if args.NodeNames == nil {
		return nil
	}

	var load map[string]int
	if isMigrationTarget(args.Pod) {
		var err error
		load, err = e.countArriving(ctx, *args.NodeNames)
		if err != nil {
			e.log.Error("Failed to count migrations, the target pod is not prioritized", logger.SlogErr(err), "pod", podKey(args.Pod))
		}
	}

	priorities := make([]HostPriority, 0, len(*args.NodeNames))
	for _, node := range *args.NodeNames {
		score := int64(0)
		if load != nil && load[node] == 0 {
			score = freeNodeScore
		}
		priorities = append(priorities, HostPriority{Host: node, Score: score})
	}
	return priorities
}

// countArriving returns the number of unfinished migrations whose target pod is bound to each
// candidate node, plus, with countOutgoing, the migrations transferring memory from it.
func (e *InboundMigrationExtender) countArriving(ctx context.Context, nodes []string) (map[string]int, error) {
	arriving := make(map[string]int, len(nodes))
	for _, node := range nodes {
		arriving[node] = 0
		var pods corev1.PodList
		if err := e.client.List(ctx, &pods, client.MatchingFields{indexer.IndexFieldPodByMigrationTargetNode: node}); err != nil {
			return nil, fmt.Errorf("list migration target pods on node %s: %w", node, err)
		}

		for i := range pods.Items {
			inFlight, err := e.isMigrationInFlight(ctx, &pods.Items[i])
			if err != nil {
				return nil, err
			}
			if inFlight {
				arriving[node]++
			}
		}
	}

	if e.countOutgoing {
		if err := e.addTransferringOutgoing(ctx, arriving); err != nil {
			return nil, err
		}
	}
	return arriving, nil
}

// addTransferringOutgoing adds to load the migrations transferring memory from each node: with a
// shared per-node budget they take the same slots as the incoming ones.
func (e *InboundMigrationExtender) addTransferringOutgoing(ctx context.Context, load map[string]int) error {
	var migrations virtv1.VirtualMachineInstanceMigrationList
	if err := e.client.List(ctx, &migrations); err != nil {
		return fmt.Errorf("list migrations: %w", err)
	}

	for i := range migrations.Items {
		migration := &migrations.Items[i]
		if migration.Status.Phase != virtv1.MigrationRunning {
			continue
		}
		var kvvmi virtv1.VirtualMachineInstance
		err := e.client.Get(ctx, types.NamespacedName{Namespace: migration.Namespace, Name: migration.Spec.VMIName}, &kvvmi)
		switch {
		case apierrors.IsNotFound(err):
			continue
		case err != nil:
			return fmt.Errorf("get the VMI of migration %s/%s: %w", migration.Namespace, migration.Name, err)
		}
		if state := kvvmi.Status.MigrationState; state != nil && state.MigrationUID == migration.UID {
			if _, candidate := load[state.SourceNode]; candidate {
				load[state.SourceNode]++
			}
		}
	}
	return nil
}

func (e *InboundMigrationExtender) isMigrationInFlight(ctx context.Context, pod *corev1.Pod) (bool, error) {
	key := types.NamespacedName{Namespace: pod.Namespace, Name: pod.Annotations[virtv1.MigrationJobNameAnnotation]}
	if key.Name == "" {
		return false, nil
	}

	var vmim virtv1.VirtualMachineInstanceMigration
	err := e.client.Get(ctx, key, &vmim)
	switch {
	case apierrors.IsNotFound(err):
		return false, nil
	case err != nil:
		return false, fmt.Errorf("get migration %s: %w", key, err)
	}

	// The name can be reused by a newer migration of the same VM: only the one that created the pod counts.
	return string(vmim.UID) == pod.Labels[virtv1.MigrationJobLabel] && !vmim.IsFinal(), nil
}

func isMigrationTarget(pod *corev1.Pod) bool {
	return pod != nil && pod.Labels[launcherAppLabel] == launcherAppName && pod.Labels[migrationJobLabel] != ""
}

func podKey(pod *corev1.Pod) string {
	if pod == nil {
		return ""
	}
	return pod.Namespace + "/" + pod.Name
}
