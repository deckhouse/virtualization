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

package service

import (
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/types"

	"github.com/deckhouse/deckhouse/pkg/log"
	"github.com/deckhouse/virtualization-controller/pkg/common"
	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/common/humanize_bytes"
	"github.com/deckhouse/virtualization-controller/pkg/common/imageformat"
	"github.com/deckhouse/virtualization-controller/pkg/common/percent"
	podutil "github.com/deckhouse/virtualization-controller/pkg/common/pod"
	"github.com/deckhouse/virtualization-controller/pkg/controller/conditions"
	"github.com/deckhouse/virtualization-controller/pkg/controller/monitoring"
	"github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type StatService struct {
	logger *log.Logger
}

func NewStatService(logger *log.Logger) *StatService {
	return &StatService{
		logger: logger.With("svc", "stat"),
	}
}

func (s StatService) GetFormat(pod *corev1.Pod) string {
	finalReport, err := monitoring.GetFinalReportFromPod(pod)
	if err != nil {
		s.logger.Error("GetFormat: Cannot get final report from pod", "err", err)
		return ""
	}

	if finalReport == nil {
		return ""
	}

	return finalReport.Format
}

func (s StatService) GetCDROM(pod *corev1.Pod) bool {
	finalReport, err := monitoring.GetFinalReportFromPod(pod)
	if err != nil {
		s.logger.Error("GetCDROM: Cannot get final report from pod", "err", err)
		return false
	}

	if finalReport == nil {
		return false
	}

	return imageformat.IsISO(finalReport.Format)
}

func (s StatService) GetSize(pod *corev1.Pod) v1alpha2.ImageStatusSize {
	finalReport, err := monitoring.GetFinalReportFromPod(pod)
	if err != nil {
		s.logger.Error("GetSize: Cannot get final report from pod", "err", err)
		return v1alpha2.ImageStatusSize{}
	}

	if finalReport == nil {
		return v1alpha2.ImageStatusSize{}
	}

	unpackedSizeBytes := resource.NewQuantity(int64(finalReport.UnpackedSizeBytes), resource.BinarySI)

	return v1alpha2.ImageStatusSize{
		Stored:        humanize_bytes.HumanizeIBytes(finalReport.StoredSizeBytes),
		StoredBytes:   strconv.FormatUint(finalReport.StoredSizeBytes, 10),
		Unpacked:      humanize_bytes.HumanizeIBytes(uint64(unpackedSizeBytes.Value())),
		UnpackedBytes: strconv.FormatInt(unpackedSizeBytes.Value(), 10),
	}
}

var (
	ErrNotInitialized        = errors.New("not initialized")
	ErrNotScheduled          = errors.New("not scheduled")
	ErrProvisioningFailed    = errors.New("provisioning failed")
	ErrDVCRNoSpaceImageError = errors.New("DVCR is out of space; please contact the cluster administrator")
	// ErrDVCRNoSpaceDiskError is intended to avoid confusion by clarifying that DVCR is needed as an intermediary when creating a virtual disk.
	ErrDVCRNoSpaceDiskError = errors.New("DVCR is out of space to create the virtual disk; please contact the cluster administrator")
)

func (s StatService) CheckPod(pod *corev1.Pod) error {
	if pod == nil {
		return errors.New("nil pod")
	}

	podInitializedCond, ok := conditions.GetPodCondition(corev1.PodInitialized, pod.Status.Conditions)
	if ok && podInitializedCond.Status == corev1.ConditionFalse {
		return fmt.Errorf("provisioning Pod %s/%s is %w: %s", pod.Namespace, pod.Name, ErrNotInitialized, podInitializedCond.Message)
	}

	podScheduledCond, ok := conditions.GetPodCondition(corev1.PodScheduled, pod.Status.Conditions)
	if ok && podScheduledCond.Status == corev1.ConditionFalse {
		return fmt.Errorf("provisioning Pod %s/%s is %w: %s", pod.Namespace, pod.Name, ErrNotScheduled, podScheduledCond.Message)
	}

	report, err := monitoring.GetFinalReportFromPod(pod)
	if err != nil && !errors.Is(err, monitoring.ErrTerminationMessageNotFound) {
		return err
	}

	if report != nil && report.ErrMessage != "" {
		if s.isDVCRNoSpaceError(report.ErrMessage) {
			if strings.HasPrefix(pod.Name, "d8v-vd-") {
				return fmt.Errorf("%w: %w", ErrProvisioningFailed, ErrDVCRNoSpaceDiskError)
			}
			return fmt.Errorf("%w: %w", ErrProvisioningFailed, ErrDVCRNoSpaceImageError)
		}
		return fmt.Errorf("%w: Pod %s/%s termination message: %s", ErrProvisioningFailed, pod.Namespace, pod.Name, report.ErrMessage)
	}

	if pod.Status.Phase == corev1.PodFailed {
		return fmt.Errorf("%w: Pod %s/%s failed", ErrProvisioningFailed, pod.Namespace, pod.Name)
	}

	return nil
}

func (s StatService) isDVCRNoSpaceError(terminationMessage string) bool {
	dvcrSvc := "dvcr.d8-virtualization.svc"

	noSpaceErrorPattern := fmt.Sprintf("Err:%d", syscall.ENOSPC)
	noDigitPattern := `\D`
	re := regexp.MustCompile(noSpaceErrorPattern + noDigitPattern)

	if strings.Contains(terminationMessage, dvcrSvc) &&
		strings.Contains(terminationMessage, http.MethodPost) &&
		re.MatchString(terminationMessage) {
		return true
	}

	return false
}

func (s StatService) GetDownloadSpeed(ownerUID types.UID, pod *corev1.Pod) *v1alpha2.StatusSpeed {
	report, err := monitoring.GetFinalReportFromPod(pod)
	if err != nil && !errors.Is(err, monitoring.ErrTerminationMessageNotFound) {
		s.logger.Error("GetDownloadSpeed: Cannot get final report from pod", "err", err)
		return nil
	}

	if report != nil {
		return &v1alpha2.StatusSpeed{
			Avg:      report.GetAverageSpeed(),
			AvgBytes: strconv.FormatUint(report.GetAverageSpeedRaw(), 10),
		}
	}

	progress, err := monitoring.GetImportProgressFromPod(string(ownerUID), pod)
	if err != nil {
		s.logger.Error("GetDownloadSpeed: Cannot get import progress from pod", "err", err)
		return nil
	}

	if progress == nil {
		return nil
	}

	return &v1alpha2.StatusSpeed{
		Avg:          progress.AvgSpeed(),
		AvgBytes:     strconv.FormatUint(progress.AvgSpeedRaw(), 10),
		Current:      progress.CurSpeed(),
		CurrentBytes: strconv.FormatUint(progress.CurSpeedRaw(), 10),
	}
}

type GetProgressOption interface {
	Apply(progress string) string
}

func NewScaleOption(low, high float64) *ScaleOption {
	return &ScaleOption{
		Low:  low,
		High: high,
	}
}

type ScaleOption struct {
	Low  float64
	High float64
}

func (o ScaleOption) Apply(progress string) string {
	return percent.ScalePercentage(progress, o.Low, o.High)
}

func (s StatService) GetProgress(ownerUID types.UID, pod *corev1.Pod, prevProgress string, opts ...GetProgressOption) string {
	if pod == nil {
		return prevProgress
	}

	if pod.Status.Phase == corev1.PodSucceeded {
		report, err := monitoring.GetFinalReportFromPod(pod)
		if err != nil {
			s.logger.Error("GetProgress: Cannot get final report from pod", "err", err)
			return prevProgress
		}

		if report.ErrMessage != "" {
			return prevProgress
		}

		res := "100%"
		for _, o := range opts {
			res = o.Apply(res)
		}

		return res
	}

	progress, err := monitoring.GetImportProgressFromPod(string(ownerUID), pod)
	if err != nil {
		s.logger.Error("GetProgress: Cannot get import progress from pod", "err", err)
		return prevProgress
	}

	if progress == nil {
		return prevProgress
	}

	res := progress.Progress()
	for _, o := range opts {
		res = o.Apply(res)
	}

	return res
}

func (s StatService) IsImportStarted(ownerUID types.UID, pod *corev1.Pod) bool {
	progress, err := monitoring.GetImportProgressFromPod(string(ownerUID), pod)
	if err != nil {
		s.logger.Error("IsImportStarted: Cannot get import progress from pod", "err", err)
		return false
	}

	if progress == nil {
		return false
	}

	return progress.ProgressRaw() > 0
}

func (s StatService) IsUploaderReady(pod *corev1.Pod, svc *corev1.Service, ing *netv1.Ingress) bool {
	if pod == nil || svc == nil || ing == nil {
		return false
	}

	ingressIsOK := ing.Annotations[annotations.AnnUploadPath] != "" || ing.Annotations[annotations.AnnUploadURLDeprecated] != ""

	return podutil.IsPodRunning(pod) && podutil.IsPodStarted(pod) && ingressIsOK
}

func (s StatService) IsUploadStarted(ownerUID types.UID, pod *corev1.Pod) bool {
	return s.IsImportStarted(ownerUID, pod)
}

func (s StatService) GetImportDuration(pod *corev1.Pod) time.Duration {
	if pod == nil {
		return 0
	}

	report, err := monitoring.GetFinalReportFromPod(pod)
	if err != nil || report == nil {
		if !errors.Is(err, monitoring.ErrTerminationMessageNotFound) {
			s.logger.Error("GetImportDuration: Cannot get final report from pod", "err", err)
		}

		return 0
	}

	return report.Duration
}

func (s StatService) GetDVCRImageName(pod *corev1.Pod) string {
	if pod == nil {
		return ""
	}

	for _, container := range pod.Spec.Containers {
		switch container.Name {
		case common.UploaderContainerName:
			for _, envVar := range container.Env {
				if envVar.Name == common.UploaderDestinationEndpoint {
					return envVar.Value
				}
			}
		case common.ImporterContainerName:
			for _, envVar := range container.Env {
				if envVar.Name == common.ImporterDestinationEndpoint {
					return envVar.Value
				}
			}
		default:
			continue
		}
	}

	return ""
}
