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

package internal

import (
	"context"
	"fmt"

	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/controller/dvcr-maintenance/postponehandler"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

const postponeFilterHandlerPrefix = "vd-"

type PostponeHandlerPreFilter struct {
	postponeHandler *postponehandler.Postpone[*v1alpha2.VirtualDisk]
}

// NewPostponeHandlerPreFilter runs postpone handler only if VirtualDisk is required to import/upload
// to DVCR first.
func NewPostponeHandlerPreFilter(postponeHandler *postponehandler.Postpone[*v1alpha2.VirtualDisk]) *PostponeHandlerPreFilter {
	return &PostponeHandlerPreFilter{
		postponeHandler: postponeHandler,
	}
}

func (h PostponeHandlerPreFilter) Handle(ctx context.Context, vd *v1alpha2.VirtualDisk) (reconcile.Result, error) {
	log := logger.FromContext(ctx).With(logger.SlogHandler(postponeFilterHandlerPrefix + h.postponeHandler.Name()))

	shouldRunDVCRClient, err := h.shouldRunDVCRClient(vd)
	if err != nil {
		return reconcile.Result{}, err
	}

	if !shouldRunDVCRClient {
		log.Debug("Ignore running handler to postpone on DVCR maintenance")
		return reconcile.Result{}, nil
	}

	return h.postponeHandler.Handle(logger.ToContext(ctx, log), vd)
}
func (h PostponeHandlerPreFilter) shouldRunDVCRClient(vd *v1alpha2.VirtualDisk) (bool, error) {
	if vd == nil || vd.Spec.DataSource == nil {
		return false, nil
	}

	switch vd.Spec.DataSource.Type {
	case v1alpha2.DataSourceTypeHTTP,
		v1alpha2.DataSourceTypeUpload,
		v1alpha2.DataSourceTypeContainerImage:
		return true, nil
	case v1alpha2.DataSourceTypeObjectRef:
		return false, nil
	}

	return false, fmt.Errorf("unknown dataSource.type %s", vd.Spec.DataSource.Type)
}
