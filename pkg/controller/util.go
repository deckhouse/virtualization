package controller

import (
	"strconv"
	"strings"

	"github.com/deckhouse/virtualization-controller/pkg/common"
	cc "github.com/deckhouse/virtualization-controller/pkg/controller/common"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	// SourceHTTP is the source type HTTP, if unspecified or invalid, it defaults to SourceHTTP
	SourceHTTP = "http"
	// SourceNone means there is no source.
	SourceNone = "none"
	// SourceContainerImage is the source type of container image.
	SourceContainerImage = "containerImage"
)

// MergeLabels adds source labels to destination (does not change existing ones)
func MergeLabels(src, dest map[string]string) map[string]string {
	if dest == nil {
		dest = map[string]string{}
	}

	for k, v := range src {
		dest[k] = v
	}

	return dest
}

// SetRecommendedLabels sets the recommended labels on CDI resources (does not get rid of existing ones)
func SetRecommendedLabels(obj metav1.Object, installerLabels map[string]string, controllerName string) {
	staticLabels := map[string]string{
		common.AppKubernetesManagedByLabel: controllerName,
		common.AppKubernetesComponentLabel: "storage",
	}

	// Merge static & existing labels
	mergedLabels := MergeLabels(staticLabels, obj.GetLabels())
	// Add installer dynamic labels as well (/version, /part-of)
	mergedLabels = MergeLabels(installerLabels, mergedLabels)

	obj.SetLabels(mergedLabels)
}

func setAnnotationsFromPodWithPrefix(anno map[string]string, pod *corev1.Pod, prefix string) {
	if pod == nil || pod.Status.ContainerStatuses == nil {
		return
	}
	annPodRestarts, _ := strconv.Atoi(anno[cc.AnnPodRestarts])
	podRestarts := int(pod.Status.ContainerStatuses[0].RestartCount)
	if podRestarts >= annPodRestarts {
		anno[cc.AnnPodRestarts] = strconv.Itoa(podRestarts)
	}
	//setVddkAnnotations(anno, pod)
	containerState := pod.Status.ContainerStatuses[0].State
	if containerState.Running != nil {
		anno[prefix] = "true"
		anno[prefix+".message"] = ""
		anno[prefix+".reason"] = PodRunningReason
	} else {
		anno[cc.AnnRunningCondition] = "false"
		if containerState.Waiting != nil && containerState.Waiting.Reason != "CrashLoopBackOff" {
			anno[prefix+".message"] = simplifyKnownMessage(containerState.Waiting.Message)
			anno[prefix+".reason"] = containerState.Waiting.Reason
		} else if containerState.Terminated != nil {
			anno[prefix+".message"] = simplifyKnownMessage(containerState.Terminated.Message)
			anno[prefix+".reason"] = containerState.Terminated.Reason
			//if strings.Contains(containerState.Terminated.Message, common.PreallocationApplied) {
			//	anno[cc.AnnPreallocationApplied] = "true"
			//}
		}
	}
}

func simplifyKnownMessage(msg string) string {
	if strings.Contains(msg, "is larger than the reported available") ||
		strings.Contains(msg, "no space left on device") ||
		strings.Contains(msg, "file largest block is bigger than maxblock") {
		return "DataVolume too small to contain image"
	}

	return msg
}
