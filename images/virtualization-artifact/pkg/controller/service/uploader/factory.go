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
	"strconv"

	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"

	"github.com/deckhouse/virtualization-controller/pkg/common"
	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/common/datasource"
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

// ciliumNetworkPolicyGVK is the CiliumNetworkPolicy type. It is built as an
// unstructured object to avoid pulling the cilium API go module into the
// controller (the same pattern used for the Gateway API HTTPRoute above).
var ciliumNetworkPolicyGVK = schema.GroupVersionKind{
	Group:   "cilium.io",
	Version: "v2",
	Kind:    "CiliumNetworkPolicy",
}

// Container ports the uploader pod serves: metrics (8443) and upload (8444).
const (
	uploaderMetricsPort = 8443
	uploaderUploadPort  = 8444
)

// ciliumNamespaceLabel is the synthesized identity label Cilium attaches to
// every pod endpoint for namespace-based matching in fromEndpoints (Cilium's
// EndpointSelector has no namespaceSelector field, unlike the standard
// NetworkPolicy peer).
const ciliumNamespaceLabel = "io.kubernetes.pod.namespace"

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
	NetworkPolicy() *unstructured.Unstructured
	Ingress(uploadPath string) *netv1.Ingress
	HTTPRoute(uploadPath string) *unstructured.Unstructured
}

type factory struct {
	sup                 supplements.Generator
	ownerReference      metav1.OwnerReference
	podSettings         PodSettings
	ingressSettings     IngressSettings
	listenerSetSettings ListenerSetSettings
	networkPolicySpec   *NetworkPolicySettings
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
	Checksums              map[string]string
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

// NetworkPolicySettings carries the source namespaces and the API-Gateway flag used
// to scope the uploader CiliumNetworkPolicy ingress in network-isolated projects.
// ControllerNamespace is where the virtualization-controller runs (metrics scrape +
// IsUploaderReady in-cluster probe); IngressNamespace is the Deckhouse ingress-nginx
// module namespace (Ingress-path upload proxying); GatewayNamespace is the Gateway
// API data-plane (alb) namespace (API-Gateway-path upload proxying); UseAPIGateway
// toggles which of IngressNamespace/GatewayNamespace applies; OwnNamespace is the
// uploader pod's namespace (in-cluster upload from pods of the same project).
type NetworkPolicySettings struct {
	ControllerNamespace string
	IngressNamespace    string
	GatewayNamespace    string
	UseAPIGateway       bool
	OwnNamespace        string
}

func NewFactory(
	sup supplements.Generator,
	podSettings PodSettings,
	ingressSettings IngressSettings,
	listenerSetSettings ListenerSetSettings,
	networkPolicySettings *NetworkPolicySettings,
	ownerReference metav1.OwnerReference,
) Factory {
	return &factory{
		sup:                 sup,
		podSettings:         podSettings,
		ingressSettings:     ingressSettings,
		listenerSetSettings: listenerSetSettings,
		networkPolicySpec:   networkPolicySettings,
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

// NetworkPolicy builds the CiliumNetworkPolicy for the uploader pod with scoped
// ingress: the virtualization-controller namespace (metrics + readiness probe),
// the ingress controller or Gateway API data-plane namespace (user upload proxying),
// the uploader pod's own namespace (in-cluster upload from pods of the same
// project), and the cluster nodes (host-network in-cluster upload from nodes).
// Egress is allow-all. The CiliumNetworkPolicy is built as an unstructured object
// to avoid a go dependency on the cilium API module (same pattern as HTTPRoute).
// It sets no finalizer: the object is owned by the resource and removed by garbage
// collection.
func (f factory) NetworkPolicy() *unstructured.Unstructured {
	name := f.sup.NetworkPolicy()

	cnp := &unstructured.Unstructured{}
	cnp.SetGroupVersionKind(ciliumNetworkPolicyGVK)
	cnp.SetName(name.Name)
	cnp.SetNamespace(name.Namespace)
	cnp.SetLabels(f.commonLabels())
	cnp.SetOwnerReferences([]metav1.OwnerReference{f.ownerReference})
	cnp.Object["spec"] = uploaderCNPSpec(f.networkPolicySpec)
	return cnp
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
	env := []corev1.EnvVar{
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

	// Upload source checksum settings.
	if checksums := datasource.FormatChecksums(f.podSettings.Checksums); checksums != "" {
		env = append(env, corev1.EnvVar{
			Name:  common.UploaderChecksums,
			Value: checksums,
		})
	}

	return env
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

// uploaderCNPSpec builds the spec of the uploader CiliumNetworkPolicy as an
// object (the CRD requires spec to be an object, not an array) of unstructured
// maps/slices (so the object can be applied without a cilium API go dependency):
//   - endpointSelector: the uploader app labels;
//   - ingress: from the controller namespace on the metrics + upload ports, from
//     the ingress controller OR the Gateway data-plane namespace on the upload
//     port (depending on UseAPIGateway), from the uploader pod's own namespace
//     on the upload port, and from the cluster nodes (host-network in-cluster
//     upload) on the upload port;
//   - egress: allow-all.
//
// Empty source namespaces skip the corresponding rule (graceful degradation):
// the upload through that path does not work, but the controller metrics scrape
// stays as long as the controller namespace is set.
func uploaderCNPSpec(s *NetworkPolicySettings) map[string]interface{} {
	spec := map[string]interface{}{
		"endpointSelector": map[string]interface{}{
			"matchExpressions": []interface{}{
				map[string]interface{}{
					"key":      annotations.AppLabel,
					"operator": "In",
					"values":   []interface{}{annotations.CDILabelValue, annotations.DVCRLabelValue},
				},
			},
		},
		// An egress rule with toEntities: [all] allows all egress (cluster pods, hosts,
		// world). The uploader pod must reach DVCR to push the uploaded image; the
		// project isolated NetworkPolicy imposes a default egress deny, so this rule
		// is required to override it (Cilium union: an allow from any policy wins).
		// Unlike the standard NetworkPolicy egress: [{}] (wildcard), a Cilium egress
		// rule with no destination matcher matches nothing — toEntities: [all] is the
		// Cilium equivalent of allow-all egress.
		"egress": []interface{}{map[string]interface{}{"toEntities": []interface{}{"all"}}},
		"enableDefaultDeny": map[string]interface{}{
			"ingress": true,
			"egress":  false,
		},
	}
	if s == nil {
		return spec
	}

	var ingressRules []interface{}
	if s.ControllerNamespace != "" {
		ingressRules = append(ingressRules,
			namespaceIngressRule(s.ControllerNamespace, uploaderMetricsPort),
			namespaceIngressRule(s.ControllerNamespace, uploaderUploadPort),
		)
	}
	// Only one of the ingress-controller / gateway namespaces applies at a time;
	// the inactive exposure does not exist, so its source is never a real client.
	if s.UseAPIGateway {
		if s.GatewayNamespace != "" {
			ingressRules = append(ingressRules, namespaceIngressRule(s.GatewayNamespace, uploaderUploadPort))
		}
	} else {
		if s.IngressNamespace != "" {
			ingressRules = append(ingressRules, namespaceIngressRule(s.IngressNamespace, uploaderUploadPort))
		}
	}
	if s.OwnNamespace != "" {
		ingressRules = append(ingressRules, namespaceIngressRule(s.OwnNamespace, uploaderUploadPort))
	}
	ingressRules = append(ingressRules, entitiesIngressRule([]string{"cluster"}, uploaderUploadPort))

	if len(ingressRules) > 0 {
		spec["ingress"] = ingressRules
	}
	return spec
}

// namespaceIngressRule builds a CiliumNetworkPolicy ingress rule allowing a
// namespace on a single TCP port.
//
// Cilium's fromEndpoints EndpointSelector has no namespaceSelector field
// (unlike the standard NetworkPolicy peer): the API server silently drops it,
// leaving a wildcard {}. Namespace filtering in Cilium is done via the
// synthesized identity label ciliumNamespaceLabel.
func namespaceIngressRule(namespace string, port int) map[string]interface{} {
	return map[string]interface{}{
		"fromEndpoints": []interface{}{
			map[string]interface{}{
				"matchLabels": map[string]interface{}{
					ciliumNamespaceLabel: namespace,
				},
			},
		},
		"toPorts": []interface{}{
			map[string]interface{}{
				"ports": []interface{}{
					map[string]interface{}{
						"port":     strconv.Itoa(port),
						"protocol": "TCP",
					},
				},
			},
		},
	}
}

// entitiesIngressRule builds a CiliumNetworkPolicy ingress rule allowing the
// given cilium entities (e.g. "cluster" for host-network traffic from nodes) on
// a single TCP port.
func entitiesIngressRule(entities []string, port int) map[string]interface{} {
	entityVals := make([]interface{}, len(entities))
	for i, e := range entities {
		entityVals[i] = e
	}
	return map[string]interface{}{
		"fromEntities": entityVals,
		"toPorts": []interface{}{
			map[string]interface{}{
				"ports": []interface{}{
					map[string]interface{}{
						"port":     strconv.Itoa(port),
						"protocol": "TCP",
					},
				},
			},
		},
	}
}
