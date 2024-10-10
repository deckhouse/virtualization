package internal

import (
	"context"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/logger"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/api/core/v1alpha2/vdcondition"
)

type InUseHandler struct {
	client client.Client
}

func NewInUseHandler(client client.Client) *InUseHandler {
	return &InUseHandler{
		client: client,
	}
}

func (h InUseHandler) Handle(ctx context.Context, vd *virtv2.VirtualDisk) (reconcile.Result, error) {
	log := logger.FromContext(ctx).With(logger.SlogHandler("inuse"))

	inUseCondition, ok := service.GetCondition(vdcondition.InUseType, vd.Status.Conditions)
	if !ok {
		inUseCondition = metav1.Condition{
			Type:   vdcondition.InUseType,
			Status: metav1.ConditionUnknown,
		}

		service.SetCondition(inUseCondition, &vd.Status.Conditions)
	}

	if inUseCondition.Status != metav1.ConditionTrue {
		var inUsed bool

		var viList virtv2.VirtualImageList
		err := h.client.List(ctx, &viList, &client.ListOptions{
			Namespace: vd.GetNamespace(),
		})

		if err != nil {
			log.Error(fmt.Sprintf("failed to list vi: %s", err))
			return reconcile.Result{}, err
		}

		for _, vi := range viList.Items {
			if vi.Spec.DataSource.Type != virtv2.DataSourceTypeObjectRef || vi.Spec.DataSource.ObjectRef == nil {
				continue
			}

			if vi.Spec.DataSource.ObjectRef.Kind != virtv2.VirtualDiskKind || vi.Spec.DataSource.ObjectRef.Name != vd.GetName() {
				continue
			}

			inUsed = true
		}

		var cviList virtv2.ClusterVirtualImageList
		err = h.client.List(ctx, &cviList, &client.ListOptions{})
		if err != nil {
			log.Error(fmt.Sprintf("failed to list cvi: %s", err))
			return reconcile.Result{}, err
		}

		for _, cvi := range cviList.Items {
			if cvi.Spec.DataSource.Type != virtv2.DataSourceTypeObjectRef || cvi.Spec.DataSource.ObjectRef == nil {
				continue
			}

			if cvi.Spec.DataSource.ObjectRef.Kind != virtv2.VirtualDiskKind || cvi.Spec.DataSource.ObjectRef.Name != vd.GetName() && cvi.Spec.DataSource.ObjectRef.Namespace != vd.GetNamespace() {
				continue
			}

			inUsed = true
		}

		if inUsed {
			inUseCondition.Status = metav1.ConditionTrue
			inUseCondition.Reason = vdcondition.InUse
			inUseCondition.Message = ""
		} else {
			inUseCondition.Status = metav1.ConditionFalse
			inUseCondition.Reason = vdcondition.NotUse
			inUseCondition.Message = ""
		}
		service.SetCondition(inUseCondition, &vd.Status.Conditions)
	}

	return reconcile.Result{}, nil
}
