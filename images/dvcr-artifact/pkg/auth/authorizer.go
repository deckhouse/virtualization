package auth

import (
	"context"

	authv1 "k8s.io/api/authorization/v1"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	authclientv1 "k8s.io/client-go/kubernetes/typed/authorization/v1"
)

type KubeAuthorizer interface {
	Authorize(ctx context.Context, username string, groups []string) (AuthorizeResult, error)
}

type AuthorizeResult struct {
	Allowed bool
	Reason  string
}

func NewKubeAuthorizer(gvr v1.GroupVersionResource, verb string, client authclientv1.SubjectAccessReviewInterface, options ...Option) (KubeAuthorizer, error) {
	a := &kubeAuthorizer{
		gvr:    gvr,
		verb:   verb,
		client: client,
	}
	for _, option := range options {
		option(a)
	}

	return a, nil
}

type Option func(*kubeAuthorizer)

func WithSubresource(subresource string) Option {
	return func(a *kubeAuthorizer) {
		a.subresource = subresource
	}
}

func WithNamespace(namespace string) Option {
	return func(a *kubeAuthorizer) {
		a.namespace = namespace
	}
}

type kubeAuthorizer struct {
	gvr  v1.GroupVersionResource
	verb string

	subresource string
	namespace   string

	client authclientv1.SubjectAccessReviewInterface
}

func (a *kubeAuthorizer) Authorize(ctx context.Context, username string, groups []string) (AuthorizeResult, error) {
	review := &authv1.SubjectAccessReview{
		Spec: authv1.SubjectAccessReviewSpec{
			User:   username,
			Groups: groups,
			ResourceAttributes: &authv1.ResourceAttributes{
				Namespace:   a.namespace,
				Verb:        a.verb,
				Group:       a.gvr.Group,
				Version:     a.gvr.Version,
				Resource:    a.gvr.Resource,
				Subresource: a.subresource,
			},
		},
	}

	result, err := a.client.Create(ctx, review, v1.CreateOptions{})
	if err != nil {
		return AuthorizeResult{
			Reason: "internal server error",
		}, err
	}

	return AuthorizeResult{
		Allowed: result.Status.Allowed,
		Reason:  result.Status.Reason,
	}, nil
}
