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

package uploader

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"

	"github.com/deckhouse/virtualization-controller/pkg/common"
	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	podutil "github.com/deckhouse/virtualization-controller/pkg/common/pod"
	"github.com/deckhouse/virtualization-controller/pkg/common/provisioner"
	"github.com/deckhouse/virtualization-controller/pkg/common/pwgen"
	"github.com/deckhouse/virtualization-controller/pkg/controller/supplements"
)

const (
	tmplUploadPath = "/upload/%s"
	uploadPath     = "/upload"

	// destinationAuthVol is the name of the volume containing DVCR docker auth config.
	destinationAuthVol = "dvcr-secret-vol"

	healthzPortName = "healthz"
	healthzPort     = 8080
	healthzPath     = "/healthz"

	// uploaderPodPort is the container port the dvcr-uploader serves uploads on.
	uploaderPodPort = 8444
)

// httpRouteGVK is the Gateway API HTTPRoute type. We use unstructured to avoid
// pulling sigs.k8s.io/gateway-api into the module for now (crude first cut).
var httpRouteGVK = schema.GroupVersionKind{
	Group:   "gateway.networking.k8s.io",
	Version: "v1",
	Kind:    "HTTPRoute",
}

// listenerSetKind is the Gateway API resource the per-upload HTTPRoutes attach to.
// The Gateway itself is owned by the alb module, whose default listeners carry a
// placeholder certificate; the module publishes the upload host through its own
// ListenerSet instead.
const listenerSetKind = "ListenerSet"

// Factory builds the set of objects that expose an uploader: the Pod, its
// Service, NetworkPolicy and either an Ingress or a Gateway API HTTPRoute. All
// objects share the same owner reference and common labels.
//
// Ingress and HTTPRoute take the upload path as an argument rather than
// generating it: the path is part of the URL handed to the user, so the caller
// owns its lifetime and reuses the already published one across reconciles.
type Factory interface {
	Pod() (*corev1.Pod, error)
	Service() *corev1.Service
	NetworkPolicy() *netv1.NetworkPolicy
	Ingress(uploadPath string) *netv1.Ingress
	HTTPRoute(uploadPath string) *unstructured.Unstructured
}

type factory struct {
	sup                 supplements.Generator
	ownerReference      metav1.OwnerReference
	podSettings         PodSettings
	ingressSettings     IngressSettings
	listenerSetSettings ListenerSetSettings
}

// PodSettings carries the uploader Pod parameters that are not derived from the
// supplements generator or the owner reference.
type PodSettings struct {
	Image                string
	PullPolicy           string
	ControllerName       string
	PriorityClassName    string
	InstallerLabels      map[string]string
	ResourceRequirements *corev1.ResourceRequirements
	ImagePullSecrets     []corev1.LocalObjectReference
	NodePlacement        *provisioner.NodePlacement

	Verbose                string
	DestinationEndpoint    string
	DestinationInsecureTLS string
	DestinationAuthSecret  string
}

// IngressSettings carries everything the Ingress exposure needs, including the
// host it publishes: the two exposures are configured independently, so neither
// borrows the other's host.
type IngressSettings struct {
	Host          string
	ClassName     *string
	TLSSecretName string
}

// ListenerSetSettings points at the ListenerSet that publishes the upload host on
// the Gateway, and carries that host. The ListenerSet is rendered by the module
// chart into the controller namespace, and every per-upload HTTPRoute attaches to
// its listener.
type ListenerSetSettings struct {
	Host         string
	Name         string
	Namespace    string
	ListenerName string
}

func NewFactory(
	sup supplements.Generator,
	podSettings PodSettings,
	ingressSettings IngressSettings,
	listenerSetSettings ListenerSetSettings,
	ownerReference metav1.OwnerReference,
) Factory {
	return &factory{
		sup:                 sup,
		podSettings:         podSettings,
		ingressSettings:     ingressSettings,
		listenerSetSettings: listenerSetSettings,
		ownerReference:      ownerReference,
	}
}

func (f factory) Pod() (*corev1.Pod, error) {
	supPod := f.sup.UploaderPod()
	supService := f.sup.UploaderService()

	labels := f.commonLabels()
	labels[annotations.UploaderServiceLabel] = supService.Name
	labels[annotations.QuotaExcludeLabel] = annotations.QuotaExcludeValue

	pod := &corev1.Pod{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Pod",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      supPod.Name,
			Namespace: supPod.Namespace,
			Annotations: map[string]string{
				annotations.AnnCreatedBy: "yes",
			},
			Labels: labels,
			OwnerReferences: []metav1.OwnerReference{
				f.ownerReference,
			},
		},
		Spec: corev1.PodSpec{
			// Container and volumes are added below.
			Containers:        []corev1.Container{},
			Volumes:           []corev1.Volume{},
			RestartPolicy:     corev1.RestartPolicyOnFailure,
			PriorityClassName: f.podSettings.PriorityClassName,
			ImagePullSecrets:  f.podSettings.ImagePullSecrets,
		},
	}

	if f.podSettings.NodePlacement != nil && len(f.podSettings.NodePlacement.Tolerations) > 0 {
		pod.Spec.Tolerations = f.podSettings.NodePlacement.Tolerations

		if err := provisioner.KeepNodePlacementTolerations(f.podSettings.NodePlacement, pod); err != nil {
			return nil, err
		}
	}

	container := f.uploaderContainer()
	f.addVolumes(pod, container)
	pod.Spec.Containers = append(pod.Spec.Containers, *container)

	annotations.SetRecommendedLabels(pod, f.podSettings.InstallerLabels, f.podSettings.ControllerName)
	podutil.SetRestrictedSecurityContext(&pod.Spec)

	return pod, nil
}

func (f factory) Service() *corev1.Service {
	supService := f.sup.UploaderService()

	return &corev1.Service{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Service",
			APIVersion: "v1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      supService.Name,
			Namespace: supService.Namespace,
			Annotations: map[string]string{
				annotations.AnnCreatedBy: "yes",
			},
			Labels: f.commonLabels(),
			OwnerReferences: []metav1.OwnerReference{
				f.ownerReference,
			},
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{
					Name:     common.UploaderPortName,
					Protocol: corev1.ProtocolTCP,
					Port:     common.UploaderPort,
					TargetPort: intstr.IntOrString{
						Type:   intstr.Int,
						IntVal: uploaderPodPort,
					},
				},
			},
			Selector: map[string]string{
				annotations.UploaderServiceLabel: supService.Name,
			},
			Type: corev1.ServiceTypeClusterIP,
		},
	}
}

// NetworkPolicy builds the egress-only NetworkPolicy for the uploader pod. It
// sets no finalizer: the object is owned by the resource and removed by garbage
// collection.
func (f factory) NetworkPolicy() *netv1.NetworkPolicy {
	name := f.sup.NetworkPolicy()

	return &netv1.NetworkPolicy{
		TypeMeta: metav1.TypeMeta{
			Kind:       "NetworkPolicy",
			APIVersion: netv1.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      name.Name,
			Namespace: name.Namespace,
			Labels:    f.commonLabels(),
			OwnerReferences: []metav1.OwnerReference{
				f.ownerReference,
			},
		},
		Spec: netv1.NetworkPolicySpec{
			PodSelector: metav1.LabelSelector{
				MatchExpressions: []metav1.LabelSelectorRequirement{
					{
						Key:      annotations.AppLabel,
						Operator: metav1.LabelSelectorOpIn,
						Values:   []string{annotations.CDILabelValue, annotations.DVCRLabelValue},
					},
				},
			},
			Egress:      []netv1.NetworkPolicyEgressRule{{}},
			PolicyTypes: []netv1.PolicyType{netv1.PolicyTypeEgress},
		},
	}
}

func (f factory) Ingress(path string) *netv1.Ingress {
	tlsEnabled := f.ingressSettings.TLSSecretName != ""
	uploadURL := uploadURL(f.ingressSettings.Host, tlsEnabled, path)

	supIngress := f.sup.UploaderIngress()
	supService := f.sup.UploaderService()

	ingress := &netv1.Ingress{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Ingress",
			APIVersion: netv1.SchemeGroupVersion.String(),
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      supIngress.Name,
			Namespace: supIngress.Namespace,
			Annotations: map[string]string{
				annotations.AnnUploadURL:                              uploadURL,
				annotations.AnnUploadPath:                             path,
				"nginx.ingress.kubernetes.io/proxy-body-size":         "0",
				"nginx.ingress.kubernetes.io/proxy-request-buffering": "off",
				"nginx.ingress.kubernetes.io/proxy-buffering":         "off",
				"nginx.ingress.kubernetes.io/rewrite-target":          uploadPath,
			},
			Labels: f.commonLabels(),
			OwnerReferences: []metav1.OwnerReference{
				f.ownerReference,
			},
		},
		Spec: netv1.IngressSpec{
			IngressClassName: f.ingressSettings.ClassName,
			Rules: []netv1.IngressRule{
				{
					Host: f.ingressSettings.Host,
					IngressRuleValue: netv1.IngressRuleValue{
						HTTP: &netv1.HTTPIngressRuleValue{
							Paths: []netv1.HTTPIngressPath{
								{
									Path:     path,
									PathType: ptr.To(netv1.PathTypeExact),
									Backend: netv1.IngressBackend{
										Service: &netv1.IngressServiceBackend{
											Name: supService.Name,
											Port: netv1.ServiceBackendPort{
												Number: common.UploaderPort,
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	if tlsEnabled {
		ingress.Spec.TLS = []netv1.IngressTLS{
			{
				Hosts:      []string{f.ingressSettings.Host},
				SecretName: f.ingressSettings.TLSSecretName,
			},
		}
		ingress.Annotations["nginx.ingress.kubernetes.io/ssl-redirect"] = "true"
	}

	return ingress
}

func (f factory) HTTPRoute(path string) *unstructured.Unstructured {
	uploadURL := uploadURL(f.listenerSetSettings.Host, true, path)

	supHTTPRoute := f.sup.UploaderHTTPRoute()
	supService := f.sup.UploaderService()

	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"spec": map[string]interface{}{
				"parentRefs": []interface{}{
					map[string]interface{}{
						"group":       httpRouteGVK.Group,
						"kind":        listenerSetKind,
						"name":        f.listenerSetSettings.Name,
						"namespace":   f.listenerSetSettings.Namespace,
						"sectionName": f.listenerSetSettings.ListenerName,
					},
				},
				"hostnames": []interface{}{f.listenerSetSettings.Host},
				"rules": []interface{}{
					map[string]interface{}{
						"matches": []interface{}{
							map[string]interface{}{
								"path": map[string]interface{}{
									"type":  "Exact",
									"value": path,
								},
							},
						},
						"filters": []interface{}{
							map[string]interface{}{
								"type": "URLRewrite",
								"urlRewrite": map[string]interface{}{
									"path": map[string]interface{}{
										"type":            "ReplaceFullPath",
										"replaceFullPath": uploadPath,
									},
								},
							},
						},
						"backendRefs": []interface{}{
							map[string]interface{}{
								"name": supService.Name,
								"port": int64(common.UploaderPort),
							},
						},
					},
				},
			},
		},
	}

	obj.SetGroupVersionKind(httpRouteGVK)
	obj.SetName(supHTTPRoute.Name)
	obj.SetNamespace(supHTTPRoute.Namespace)
	obj.SetLabels(f.commonLabels())
	obj.SetAnnotations(map[string]string{
		annotations.AnnUploadURL:  uploadURL,
		annotations.AnnUploadPath: path,
	})
	obj.SetOwnerReferences([]metav1.OwnerReference{f.ownerReference})

	return obj
}

func (f factory) uploaderContainer() *corev1.Container {
	container := &corev1.Container{
		Name:            common.UploaderContainerName,
		Image:           f.podSettings.Image,
		ImagePullPolicy: corev1.PullPolicy(f.podSettings.PullPolicy),
		Command:         []string{"/usr/local/bin/dvcr-uploader"},
		Args:            []string{"-v=" + f.podSettings.Verbose},
		Ports: []corev1.ContainerPort{
			{
				Name:          common.UploaderPortName,
				ContainerPort: uploaderPodPort,
				Protocol:      corev1.ProtocolTCP,
			},
			{
				Name:          healthzPortName,
				ContainerPort: healthzPort,
				Protocol:      corev1.ProtocolTCP,
			},
			{
				Name:          "metrics",
				ContainerPort: 8443,
				Protocol:      corev1.ProtocolTCP,
			},
		},
		Env: f.uploaderContainerEnv(),
		SecurityContext: &corev1.SecurityContext{
			ReadOnlyRootFilesystem: ptr.To(true),
		},
		ReadinessProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				HTTPGet: &corev1.HTTPGetAction{
					Path: healthzPath,
					Port: intstr.IntOrString{
						Type:   intstr.String,
						StrVal: healthzPortName,
					},
				},
			},
			InitialDelaySeconds: 5,
		},
	}

	if f.podSettings.ResourceRequirements != nil {
		container.Resources = *f.podSettings.ResourceRequirements
	}

	return container
}

func (f factory) uploaderContainerEnv() []corev1.EnvVar {
	return []corev1.EnvVar{
		{
			Name:  common.OwnerUID,
			Value: string(f.ownerReference.UID),
		},
		{
			Name:  common.UploaderDestinationEndpoint,
			Value: f.podSettings.DestinationEndpoint,
		},
		{
			Name:  common.DestinationInsecureTLSVar,
			Value: f.podSettings.DestinationInsecureTLS,
		},
	}
}

// addVolumes fills Volumes in the Pod spec and VolumeMounts/envs in the container spec.
func (f factory) addVolumes(pod *corev1.Pod, container *corev1.Container) {
	podutil.AddEmptyDirVolume(pod, container, "tmp", "/tmp")

	if f.podSettings.DestinationAuthSecret != "" {
		// Mount DVCR auth Secret and pass directory with mounted DVCR login config.
		podutil.AddVolume(pod, container,
			podutil.CreateSecretVolume(destinationAuthVol, f.podSettings.DestinationAuthSecret),
			podutil.CreateVolumeMount(destinationAuthVol, common.UploaderDestinationAuthConfigDir),
			corev1.EnvVar{
				Name:  common.UploaderDestinationAuthConfigVar,
				Value: common.UploaderDestinationAuthConfigFile,
			},
		)
	}
}

// commonLabels returns the labels shared by every object the factory builds.
func (f factory) commonLabels() map[string]string {
	return map[string]string{
		annotations.HeritageLabel: annotations.HeritageValue,
		annotations.AppLabel:      annotations.DVCRLabelValue,
	}
}

// GenerateUploadPath returns a fresh random upload path for a new exposure.
func GenerateUploadPath() string {
	return fmt.Sprintf(tmplUploadPath, pwgen.AlphaNum(32))
}

// uploadURL builds the public upload URL. The host is always set: the controller
// rejects an empty upload host at startup (see config.LoadDVCRSettingsFromEnvs),
// so the URL never degrades into a hostless one.
func uploadURL(host string, withTLS bool, path string) string {
	scheme := "http"
	if withTLS {
		scheme = "https"
	}
	return fmt.Sprintf("%s://%s%s", scheme, host, path)
}
