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

// Package uploader reconciles the set of objects that expose an image uploader:
// the Factory (factory.go) builds the desired specs without touching the API,
// and the Uploader service applies them. Unlike the legacy
// service.UploaderService it does not use finalizers — all objects are owned by
// the resource and garbage-collected — and exposes an idempotent Apply instead
// of Start.
package uploader

import (
	"context"
	"fmt"
	"reflect"

	corev1 "k8s.io/api/core/v1"
	netv1 "k8s.io/api/networking/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/component-base/featuregate"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/common/annotations"
	"github.com/deckhouse/virtualization-controller/pkg/common/datasource"
	"github.com/deckhouse/virtualization-controller/pkg/common/object"
	"github.com/deckhouse/virtualization-controller/pkg/common/provisioner"
	"github.com/deckhouse/virtualization-controller/pkg/controller/service"
	"github.com/deckhouse/virtualization-controller/pkg/controller/supplements"
	"github.com/deckhouse/virtualization-controller/pkg/dvcr"
	"github.com/deckhouse/virtualization-controller/pkg/dvcr/registrytoken"
	"github.com/deckhouse/virtualization-controller/pkg/featuregates"
)

// fieldOwner is the server-side apply field manager for objects this service
// keeps reconciled.
const fieldOwner = "virtualization-controller"

// Uploader reconciles the set of objects that expose an image uploader.
type Uploader interface {
	// Apply idempotently ensures the uploader Pod, its Service, NetworkPolicy,
	// supplements and the external exposure (Ingress or HTTPRoute) exist.
	Apply(ctx context.Context, obj client.Object, sup supplements.Generator, settings Settings, caBundle *datasource.CABundle, opts ...Option) error
	// EnsureExposure restores the external exposure of an already running uploader
	// when it drifted from the desired state. Safe to call on every reconcile.
	EnsureExposure(ctx context.Context, obj client.Object, sup supplements.Generator) error
	GetPod(ctx context.Context, sup supplements.Generator) (*corev1.Pod, error)
	GetService(ctx context.Context, sup supplements.Generator) (*corev1.Service, error)
	GetExposure(ctx context.Context, sup supplements.Generator) (UploaderExposure, error)
	GetInClusterURL(svc *corev1.Service) string
	// Cleanup deletes the uploader objects and returns whether something was
	// deleted along with a human-readable reason describing what it waits for.
	Cleanup(ctx context.Context, sup supplements.Generator) (bool, string, error)
}

type uploaderService struct {
	client         client.Client
	dvcrSettings   *dvcr.Settings
	image          string
	pullPolicy     string
	verbose        string
	controllerName string
	requirements   corev1.ResourceRequirements
	featureGate    featuregate.FeatureGate
}

func NewUploader(
	c client.Client,
	dvcrSettings *dvcr.Settings,
	image string,
	requirements corev1.ResourceRequirements,
	pullPolicy string,
	verbose string,
	controllerName string,
	featureGate featuregate.FeatureGate,
) Uploader {
	return &uploaderService{
		client:         c,
		dvcrSettings:   dvcrSettings,
		image:          image,
		requirements:   requirements,
		pullPolicy:     pullPolicy,
		verbose:        verbose,
		controllerName: controllerName,
		featureGate:    featureGate,
	}
}

// Option configures a single Apply call.
type Option func(*options)

type options struct {
	nodePlacement *provisioner.NodePlacement
}

func newOptions(opts ...Option) *options {
	o := &options{}
	for _, opt := range opts {
		opt(o)
	}
	return o
}

// WithNodePlacement schedules the uploader pod with the given node placement.
func WithNodePlacement(nodePlacement *provisioner.NodePlacement) Option {
	return func(o *options) {
		o.nodePlacement = nodePlacement
	}
}

// WithSystemNodeToleration schedules the uploader pod onto system nodes.
func WithSystemNodeToleration() Option {
	return func(o *options) {
		if o.nodePlacement == nil {
			o.nodePlacement = &provisioner.NodePlacement{}
		}
		provisioner.AddTolerationForSystemNodes(o.nodePlacement)
	}
}

func (u *uploaderService) Apply(ctx context.Context, obj client.Object, sup supplements.Generator, settings Settings, caBundle *datasource.CABundle, opts ...Option) error {
	o := newOptions(opts...)
	ownerRef := metav1.NewControllerRef(obj, obj.GetObjectKind().GroupVersionKind())

	f := u.newFactory(sup, *ownerRef, settings, o.nodePlacement)

	pod, err := f.Pod()
	if err != nil {
		return fmt.Errorf("build uploader pod: %w", err)
	}
	pod, err = createOrGet(ctx, u.client, pod)
	if err != nil {
		return fmt.Errorf("ensure uploader pod: %w", err)
	}

	if err = supplements.EnsureForPod(ctx, u.client, sup, pod, caBundle, u.dvcrSettings, u.tokenScope(settings)); err != nil {
		return fmt.Errorf("ensure pod supplements: %w", err)
	}

	if err = u.createIfAbsent(ctx, f.NetworkPolicy()); err != nil {
		return fmt.Errorf("ensure network policy: %w", err)
	}

	if err = u.createIfAbsent(ctx, f.Service()); err != nil {
		return fmt.Errorf("ensure uploader service: %w", err)
	}

	// Without a public host there is nothing to publish the upload on, so no
	// external exposure is created and the upload stays reachable through the
	// in-cluster Service URL only.
	if !u.externalExposureRequired() {
		return nil
	}

	uploadPath, err := u.uploadPath(ctx, sup)
	if err != nil {
		return fmt.Errorf("resolve upload path: %w", err)
	}

	if u.useAPIGateway() {
		if err = u.validateAPIGatewaySettings(); err != nil {
			return err
		}
		if err = u.apply(ctx, f.HTTPRoute(uploadPath)); err != nil {
			return fmt.Errorf("ensure uploader httproute: %w", err)
		}
	} else {
		ing := f.Ingress(uploadPath)
		if err = u.apply(ctx, ing); err != nil {
			return fmt.Errorf("ensure uploader ingress: %w", err)
		}
		// The Ingress needs the TLS secret copied into its namespace; the HTTPRoute
		// does not (TLS terminates on the shared Gateway).
		if err = supplements.EnsureForIngress(ctx, u.client, sup, ing, u.dvcrSettings); err != nil {
			return err
		}
	}

	return nil
}

// EnsureExposure restores the external exposure when it drifted from the desired
// state. All uploaders share one public host: if the Ingress host drifts (e.g.
// after publicDomainTemplate changed) or its copied TLS secret goes missing,
// ingress-nginx serves its default certificate for the whole host and every
// upload on it breaks. IsUploaderReady HTTPS-probes that host, so both have to be
// restored. Drift is detected against the cached object, so a steady-state
// reconcile issues no writes.
//
// Creating the exposure from scratch is Apply's job: this is a no-op while there
// is nothing to reconcile yet.
func (u *uploaderService) EnsureExposure(ctx context.Context, obj client.Object, sup supplements.Generator) error {
	if !u.externalExposureRequired() {
		return nil
	}

	drifted, uploadPath, err := u.exposureDrifted(ctx, sup)
	if err != nil {
		return err
	}
	if !drifted {
		return nil
	}

	// The exposure lost the annotation that records the path it publishes, so the
	// path cannot be preserved: it is regenerated instead. An empty path is invalid
	// for both an Ingress rule and an HTTPRoute match, and the apply below writes
	// the new one into the object and its AnnUploadURL annotation at once, so the
	// URL the user is given stays in sync with what is actually served.
	if uploadPath == "" {
		uploadPath = GenerateUploadPath()
	}

	ownerRef := metav1.NewControllerRef(obj, obj.GetObjectKind().GroupVersionKind())
	f := u.newFactory(sup, *ownerRef, Settings{}, nil)

	if u.useAPIGateway() {
		if err = u.validateAPIGatewaySettings(); err != nil {
			return err
		}
		if err = u.apply(ctx, f.HTTPRoute(uploadPath)); err != nil {
			return fmt.Errorf("reconcile uploader httproute: %w", err)
		}
		return nil
	}

	ing := f.Ingress(uploadPath)
	if err = u.apply(ctx, ing); err != nil {
		return fmt.Errorf("reconcile uploader ingress: %w", err)
	}
	if err = supplements.EnsureForIngress(ctx, u.client, sup, ing, u.dvcrSettings); err != nil {
		return err
	}

	return nil
}

// exposureDrifted reports whether the existing exposure diverged from the desired
// host, lost the annotation recording the path it publishes, or lost the TLS secret
// copy it is probed with. It returns the upload path currently published so a repair
// keeps the URL path intact, and an empty one when the exposure no longer records it.
func (u *uploaderService) exposureDrifted(ctx context.Context, sup supplements.Generator) (bool, string, error) {
	if u.useAPIGateway() {
		route, err := u.getHTTPRoute(ctx, sup)
		if err != nil || route == nil {
			return false, "", err
		}

		hostnames, _, err := unstructured.NestedStringSlice(route.Object, "spec", "hostnames")
		if err != nil {
			return false, "", fmt.Errorf("read httproute hostnames: %w", err)
		}

		expectedHost := u.dvcrSettings.UploaderListenerSetSettings.Host
		uploadPath := route.GetAnnotations()[annotations.AnnUploadPath]
		return uploadPath == "" || len(hostnames) == 0 || hostnames[0] != expectedHost, uploadPath, nil
	}

	expectedHost := u.dvcrSettings.UploaderIngressSettings.Host

	ing, err := u.getIngress(ctx, sup)
	if err != nil || ing == nil {
		return false, "", err
	}

	// An exposure without the upload-path annotation is out of contract: the path it
	// serves is no longer known, so it is rebuilt instead of patched in place.
	uploadPath := ing.Annotations[annotations.AnnUploadPath]
	if uploadPath == "" || len(ing.Spec.Rules) == 0 || ing.Spec.Rules[0].Host != expectedHost {
		return true, uploadPath, nil
	}

	if supplements.ShouldCopyUploaderTLSSecret(u.dvcrSettings, sup) {
		tlsSecret, err := supplements.GetTLSSecret(ctx, u.client, sup)
		if err != nil {
			return false, "", err
		}
		if tlsSecret == nil {
			return true, uploadPath, nil
		}
	}

	return false, uploadPath, nil
}

// uploadPath returns the path already published in the external exposure so the
// URL handed to the user stays stable across reconciles, and a fresh random path
// when there is no exposure yet.
func (u *uploaderService) uploadPath(ctx context.Context, sup supplements.Generator) (string, error) {
	exposure, err := u.GetExposure(ctx, sup)
	if err != nil {
		return "", err
	}
	if exposure.UploadPath != "" {
		return exposure.UploadPath, nil
	}
	return GenerateUploadPath(), nil
}

func (u *uploaderService) GetPod(ctx context.Context, sup supplements.Generator) (*corev1.Pod, error) {
	pod := &corev1.Pod{}
	return supplements.FetchSupplement(ctx, u.client, sup, supplements.SupplementUploaderPod, pod)
}

func (u *uploaderService) GetService(ctx context.Context, sup supplements.Generator) (*corev1.Service, error) {
	svc := &corev1.Service{}
	return supplements.FetchSupplement(ctx, u.client, sup, supplements.SupplementUploaderService, svc)
}

func (u *uploaderService) GetInClusterURL(svc *corev1.Service) string {
	if svc == nil || svc.Spec.ClusterIP == "" {
		return ""
	}
	return fmt.Sprintf("http://%s/upload", svc.Spec.ClusterIP)
}

// UploaderExposure is an aggregated, transport-agnostic view of the uploader's
// external exposure. It unifies the knowledge carried by the Ingress and the
// Gateway API HTTPRoute so downstream code does not need to know which one backs
// the uploader.
type UploaderExposure struct {
	// Required reports whether an external exposure is expected at all. It is not
	// when no public host is configured (a cluster without
	// global.modules.publicDomainTemplate): the upload is then reachable only
	// through the in-cluster Service URL.
	Required bool
	// Exists reports whether the backing object (Ingress or HTTPRoute) exists.
	Exists bool
	// UploadURL is the public upload URL (AnnUploadURL annotation).
	UploadURL string
	// UploadPath is the upload path (AnnUploadPath annotation).
	UploadPath string
	// TLSSecret is the TLS secret used to probe the external HTTPS endpoint on the
	// Ingress path. It is nil on the Gateway path, where TLS terminates on the
	// shared Gateway and no per-namespace secret is copied.
	TLSSecret *corev1.Secret
}

// Ensured reports whether the external exposure is in its desired state: either
// its backing object exists, or no external exposure is expected at all.
func (e UploaderExposure) Ensured() bool {
	return !e.Required || e.Exists
}

// GetExposure builds the transport-agnostic view of the external exposure from
// the HTTPRoute or the Ingress depending on the active mode.
func (u *uploaderService) GetExposure(ctx context.Context, sup supplements.Generator) (UploaderExposure, error) {
	required := u.externalExposureRequired()

	if u.useAPIGateway() {
		route, err := u.getHTTPRoute(ctx, sup)
		if err != nil {
			return UploaderExposure{}, err
		}

		if route == nil {
			return UploaderExposure{Required: required}, nil
		}

		tlsSecret, err := u.getListenerTLSSecret(ctx)
		if err != nil {
			return UploaderExposure{}, err
		}

		anns := route.GetAnnotations()
		return UploaderExposure{
			Required:   required,
			Exists:     true,
			UploadURL:  anns[annotations.AnnUploadURL],
			UploadPath: anns[annotations.AnnUploadPath],
			TLSSecret:  tlsSecret,
		}, nil
	}

	ing, err := u.getIngress(ctx, sup)
	if err != nil {
		return UploaderExposure{}, err
	}

	if ing == nil {
		return UploaderExposure{Required: required}, nil
	}

	tlsSecret, err := supplements.GetTLSSecret(ctx, u.client, sup)
	if err != nil {
		return UploaderExposure{}, err
	}

	return UploaderExposure{
		Required:   required,
		Exists:     true,
		UploadURL:  ing.Annotations[annotations.AnnUploadURL],
		UploadPath: ing.Annotations[annotations.AnnUploadPath],
		TLSSecret:  tlsSecret,
	}, nil
}

func (u *uploaderService) Cleanup(ctx context.Context, sup supplements.Generator) (requeue bool, reason string, err error) {
	pod, err := u.GetPod(ctx, sup)
	if err != nil {
		return false, "", err
	}
	svc, err := u.GetService(ctx, sup)
	if err != nil {
		return false, "", err
	}
	ing, err := u.getIngress(ctx, sup)
	if err != nil {
		return false, "", err
	}
	route, err := u.getHTTPRoute(ctx, sup)
	if err != nil {
		return false, "", err
	}

	npName := sup.NetworkPolicy()
	np := &netv1.NetworkPolicy{ObjectMeta: metav1.ObjectMeta{Name: npName.Name, Namespace: npName.Namespace}}

	targets := []struct {
		role string
		obj  client.Object
	}{
		{service.CleanUpRoleUploaderPod, pod},
		{service.CleanUpRoleUploaderService, svc},
		{service.CleanUpRoleUploaderIngress, ing},
		{service.CleanUpRoleUploaderHTTPRoute, route},
		{service.CleanUpRoleNetworkPolicy, np},
	}

	var reasons []string
	for _, target := range targets {
		deleted, err := u.deleteIfPresent(ctx, target.obj)
		if err != nil {
			return false, "", err
		}
		if deleted {
			reasons = append(reasons, service.CleanUpReasonForObject(target.role, target.obj))
		}
	}

	reason = service.MergeCleanUpReasons(reasons...)

	return reason != "", reason, nil
}

func (u *uploaderService) newFactory(sup supplements.Generator, ownerRef metav1.OwnerReference, settings Settings, np *provisioner.NodePlacement) Factory {
	podSettings := PodSettings{
		Image:                u.image,
		PullPolicy:           u.pullPolicy,
		ControllerName:       u.controllerName,
		InstallerLabels:      map[string]string{},
		ResourceRequirements: &u.requirements,
		NodePlacement:        np,

		Verbose:                u.verbose,
		DestinationEndpoint:    settings.DestinationEndpoint,
		DestinationInsecureTLS: settings.DestinationInsecureTLS,
		DestinationAuthSecret:  settings.DestinationAuthSecret,
	}

	return NewFactory(sup, podSettings, u.ingressSettings(sup), u.listenerSetSettings(), ownerRef)
}

func (u *uploaderService) listenerSetSettings() ListenerSetSettings {
	s := u.dvcrSettings.UploaderListenerSetSettings
	return ListenerSetSettings{
		Host:         s.Host,
		Name:         s.Name,
		Namespace:    s.Namespace,
		ListenerName: s.ListenerName,
	}
}

func (u *uploaderService) validateAPIGatewaySettings() error {
	s := u.listenerSetSettings()
	if s.Host == "" || s.Name == "" || s.Namespace == "" || s.ListenerName == "" || u.dvcrSettings.UploaderListenerSetSettings.TLSSecretName == "" {
		return fmt.Errorf("the %s feature gate is enabled but the uploader ListenerSet is not configured", featuregates.UploadViaAPIGateway)
	}
	return nil
}

func (u *uploaderService) ingressSettings(sup supplements.Generator) IngressSettings {
	secretName := u.dvcrSettings.UploaderIngressSettings.TLSSecret
	if supplements.ShouldCopyUploaderTLSSecret(u.dvcrSettings, sup) {
		secretName = sup.UploaderTLSSecretForIngress().Name
	}

	var class *string
	if c := u.dvcrSettings.UploaderIngressSettings.Class; c != "" {
		class = &c
	}

	return IngressSettings{
		Host:          u.dvcrSettings.UploaderIngressSettings.Host,
		ClassName:     class,
		TLSSecretName: secretName,
	}
}

func (u *uploaderService) useAPIGateway() bool {
	return u.featureGate.Enabled(featuregates.UploadViaAPIGateway)
}

// externalExposureRequired reports whether the active exposure has a public host
// to publish the upload on. Clusters without global.modules.publicDomainTemplate
// have none: there the uploader is exposed by its Service only, and creating an
// Ingress or an HTTPRoute would produce a catch-all rule and a malformed upload
// URL instead of a usable endpoint.
func (u *uploaderService) externalExposureRequired() bool {
	if u.useAPIGateway() {
		return u.dvcrSettings.UploaderListenerSetSettings.Host != ""
	}

	return u.dvcrSettings.UploaderIngressSettings.Host != ""
}

func (u *uploaderService) tokenScope(settings Settings) []registrytoken.Access {
	return []registrytoken.Access{
		{
			Type:    "repository",
			Name:    u.dvcrSettings.RepoPath(settings.DestinationEndpoint),
			Actions: []string{"pull", "push"},
		},
	}
}

func (u *uploaderService) getIngress(ctx context.Context, sup supplements.Generator) (*netv1.Ingress, error) {
	ing := &netv1.Ingress{}
	return supplements.FetchSupplement(ctx, u.client, sup, supplements.SupplementUploaderIngress, ing)
}

// createOrGet creates obj and, if it already exists, fetches the current one
// into obj. It mirrors createIfAbsent but returns the live object.
func createOrGet[T client.Object](ctx context.Context, c client.Client, obj T) (T, error) {
	err := c.Create(ctx, obj)
	if err == nil {
		return obj, nil
	}
	if k8serrors.IsAlreadyExists(err) {
		err = c.Get(ctx, client.ObjectKeyFromObject(obj), obj)
		return obj, err
	}
	var zero T
	return zero, err
}

func (u *uploaderService) createIfAbsent(ctx context.Context, obj client.Object) error {
	return client.IgnoreAlreadyExists(u.client.Create(ctx, obj))
}

// apply reconciles obj with server-side apply: it creates the object if absent
// and restores spec/annotations when they drift. All uploaders share one public
// host, so a drifted Ingress host (e.g. after publicDomainTemplate changed)
// makes ingress-nginx serve its default certificate for the whole host and
// breaks every upload on it. Field ownership is stable across restarts.
func (u *uploaderService) apply(ctx context.Context, obj client.Object) error {
	return u.client.Patch(ctx, obj, client.Apply, client.FieldOwner(fieldOwner))
}

func (u *uploaderService) getHTTPRoute(ctx context.Context, sup supplements.Generator) (*unstructured.Unstructured, error) {
	route := &unstructured.Unstructured{}
	route.SetGroupVersionKind(httpRouteGVK)
	if err := u.client.Get(ctx, sup.UploaderHTTPRoute(), route); err != nil {
		if k8serrors.IsNotFound(err) || meta.IsNoMatchError(err) {
			return nil, nil
		}
		return nil, err
	}
	return route, nil
}

// getListenerTLSSecret fetches the certificate the ListenerSet terminates TLS
// with. It lives next to the ListenerSet in the controller namespace and is used
// to trust the public endpoint when probing it for readiness.
func (u *uploaderService) getListenerTLSSecret(ctx context.Context) (*corev1.Secret, error) {
	s := u.dvcrSettings.UploaderListenerSetSettings
	if s.TLSSecretName == "" {
		return nil, nil
	}

	return object.FetchObject(ctx,
		types.NamespacedName{
			Name:      s.TLSSecretName,
			Namespace: s.Namespace,
		},
		u.client,
		&corev1.Secret{},
	)
}

func (u *uploaderService) deleteIfPresent(ctx context.Context, obj client.Object) (bool, error) {
	// obj may be a typed nil (e.g. a *corev1.Pod that was never found), which is
	// not == nil as an interface, so check the underlying value too.
	if obj == nil || reflect.ValueOf(obj).IsNil() {
		return false, nil
	}
	if err := u.client.Delete(ctx, obj); err != nil {
		if k8serrors.IsNotFound(err) || meta.IsNoMatchError(err) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}
