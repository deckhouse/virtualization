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

package watcher

import (
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/source"

	"github.com/deckhouse/virtualization-controller/pkg/controller/vdexport/dataexport"
)

type DataExportWatcher struct {
	Enabled bool
}

func NewDataExportWatcher(enabled bool) *DataExportWatcher {
	return &DataExportWatcher{
		Enabled: enabled,
	}
}

func (w *DataExportWatcher) Watch(mgr manager.Manager, ctr controller.Controller) error {
	if !w.Enabled {
		return nil
	}

	if err := ctr.Watch(source.Kind(mgr.GetCache(), dataexport.NewEmptyDataExport().Unstructured),
		&handler.EnqueueRequestForObject{},
		predicate.Funcs{},
	); err != nil {
		return fmt.Errorf("error setting watch on DataExport: %w", err)
	}

	return nil
}
