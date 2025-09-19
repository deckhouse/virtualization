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

package migration

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"k8s.io/apimachinery/pkg/types"
	virtv1 "kubevirt.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/deckhouse/pkg/log"
	"github.com/deckhouse/virtualization-controller/pkg/common/patch"
	"github.com/deckhouse/virtualization-controller/pkg/controller/kvbuilder"
)

const (
	// https://github.com/qemu/qemu/commit/75997e182b695f2e3f0a2d649734952af5caf3ee
	qemuMaxLength36ControllerName = "qemu-max-length-36"
)

func newQEMUMaxLength36(client client.Client, logger *log.Logger) (Migration, error) {
	return &qemuMaxLength36{
		client: client,
		logger: logger,
	}, nil
}

type qemuMaxLength36 struct {
	client client.Client
	logger *log.Logger
}

func (r *qemuMaxLength36) Name() string {
	return qemuMaxLength36ControllerName
}

func (r *qemuMaxLength36) Migrate(ctx context.Context) error {
	disks, err := newDiskCache(ctx, r.client)
	if err != nil {
		return fmt.Errorf("failed to create diskCache: %w", err)
	}

	kvvmList := &virtv1.VirtualMachineList{}
	err = r.client.List(ctx, kvvmList)
	if err != nil {
		return err
	}

	for i := range kvvmList.Items {
		kvvm := &kvvmList.Items[i]

		needUpdate, genPatch, err := r.genPatch("/spec/template/spec", kvvm.GetNamespace(), &kvvm.Spec.Template.Spec, disks)
		if err != nil {
			return err
		}
		if !needUpdate {
			continue
		}

		r.logger.Info("Patch kvvm", slog.String("name", kvvm.Name), slog.String("namespace", kvvm.Namespace))

		if r.logger.GetLevel() <= log.LevelDebug {
			if data, err := genPatch.Data(kvvm); err == nil {
				r.logger.Debug("Patch kvvm",
					slog.String("name", kvvm.Name),
					slog.String("namespace", kvvm.Namespace),
					slog.String("data", string(data)),
				)
			}
		}

		if err = r.client.Patch(ctx, kvvm, genPatch); err != nil {
			return err
		}
	}

	return nil
}

func (r *qemuMaxLength36) genPatch(base, namespace string, spec *virtv1.VirtualMachineInstanceSpec, disks diskCache) (bool, client.Patch, error) {
	var ops []patch.JSONPatchOperation
	for i, d := range spec.Domain.Devices.Disks {
		if d.Disk == nil {
			continue
		}

		var (
			uid   types.UID
			found bool
		)

		switch {
		case strings.HasPrefix(d.Name, kvbuilder.CVIDiskPrefix):
			newName := strings.TrimPrefix(d.Name, kvbuilder.CVIDiskPrefix)
			if uid, found = disks.CVINameUID[newName]; !found {
				continue
			}
		case strings.HasPrefix(d.Name, kvbuilder.VIDiskPrefix):
			newName := strings.TrimPrefix(d.Name, kvbuilder.VIDiskPrefix)
			if uid, found = disks.VINameUID[types.NamespacedName{
				Name:      newName,
				Namespace: namespace,
			}]; !found {
				continue
			}
		case strings.HasPrefix(d.Name, kvbuilder.VDDiskPrefix):
			newName := strings.TrimPrefix(d.Name, kvbuilder.VDDiskPrefix)
			if uid, found = disks.VDNameUID[types.NamespacedName{
				Name:      newName,
				Namespace: namespace,
			}]; !found {
				continue
			}
		default:
			continue
		}

		newSerial := kvbuilder.GenerateSerial(string(uid))

		if d.Serial != "" && d.Serial != newSerial {
			ops = append(ops, patch.NewJSONPatchOperation(
				patch.PatchReplaceOp,
				fmt.Sprintf("%s/domain/devices/disks/%d/serial", base, i),
				newSerial,
			))
		}
	}
	if len(ops) == 0 {
		return false, nil, nil
	}
	bytes, err := patch.NewJSONPatch(ops...).Bytes()
	if err != nil {
		return false, nil, err
	}
	return true, client.RawPatch(types.JSONPatchType, bytes), nil
}
