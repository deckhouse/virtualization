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
	"log/slog"
	"strconv"
	"time"

	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/types"

	"github.com/deckhouse/virtualization-controller/pkg/common"
	cc "github.com/deckhouse/virtualization-controller/pkg/controller/common"
	"github.com/deckhouse/virtualization-controller/pkg/controller/monitoring"
	"github.com/deckhouse/virtualization-controller/pkg/imageformat"
	"github.com/deckhouse/virtualization-controller/pkg/util"
	virtv2 "github.com/deckhouse/virtualization/api/core/v1alpha2"
)

type StatService struct {
	logger *slog.Logger
}

func NewStatService(logger *slog.Logger) *StatService {
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

func (s StatService) GetSize(pod *corev1.Pod) virtv2.ImageStatusSize {
	finalReport, err := monitoring.GetFinalReportFromPod(pod)
	if err != nil {
		s.logger.Error("GetSize: Cannot get final report from pod", "err", err)
		return virtv2.ImageStatusSize{}
	}

	if finalReport == nil {
		return virtv2.ImageStatusSize{}
	}

	unpackedSizeBytes := resource.NewQuantity(int64(finalReport.UnpackedSizeBytes), resource.BinarySI)

	return virtv2.ImageStatusSize{
		Stored:        util.HumanizeIBytes(finalReport.StoredSizeBytes),
		StoredBytes:   strconv.FormatUint(finalReport.StoredSizeBytes, 10),
		Unpacked:      util.HumanizeIBytes(uint64(unpackedSizeBytes.Value())),
		UnpackedBytes: strconv.FormatInt(unpackedSizeBytes.Value(), 10),
	}
}

var (
	ErrNotInitialized     = errors.New("not initialized")
	ErrNotScheduled       = errors.New("not scheduled")
	ErrProvisioningFailed = errors.New("provisioning failed")
)

func (s StatService) CheckPod(pod *corev1.Pod) error {
	if pod == nil {
		return errors.New("nil pod")
	}

	podInitializedCond, ok := GetPodCondition(corev1.PodInitialized, pod.Status.Conditions)
	if ok && podInitializedCond.Status == corev1.ConditionFalse {
		return fmt.Errorf("provisioning Pod %s/%s is %w: %s", pod.Namespace, pod.Name, ErrNotInitialized, podInitializedCond.Message)
	}

	podScheduledCond, ok := GetPodCondition(corev1.PodScheduled, pod.Status.Conditions)
	if ok && podScheduledCond.Status == corev1.ConditionFalse {
		return fmt.Errorf("provisioning Pod %s/%s is %w: %s", pod.Namespace, pod.Name, ErrNotScheduled, podScheduledCond.Message)
	}

	report, err := monitoring.GetFinalReportFromPod(pod)
	if err != nil && !errors.Is(err, monitoring.ErrTerminationMessageNotFound) {
		return err
	}

	if report != nil && report.ErrMessage != "" {
		return fmt.Errorf("%w: Pod %s/%s termination message: %s", ErrProvisioningFailed, pod.Namespace, pod.Name, report.ErrMessage)
	}

	if pod.Status.Phase == corev1.PodFailed {
		return fmt.Errorf("%w: Pod %s/%s failed", ErrProvisioningFailed, pod.Namespace, pod.Name)
	}

	return nil
}

func (s StatService) GetDownloadSpeed(ownerUID types.UID, pod *corev1.Pod) *virtv2.StatusSpeed {
	report, err := monitoring.GetFinalReportFromPod(pod)
	if err != nil && !errors.Is(err, monitoring.ErrTerminationMessageNotFound) {
		s.logger.Error("GetDownloadSpeed: Cannot get final report from pod", "err", err)
		return nil
	}

	if report != nil {
		return &virtv2.StatusSpeed{
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

	return &virtv2.StatusSpeed{
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
	return common.ScalePercentage(progress, o.Low, o.High)
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

	return cc.IsPodRunning(pod) && cc.IsPodStarted(pod) && ing.Annotations[cc.AnnUploadURL] != ""
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
