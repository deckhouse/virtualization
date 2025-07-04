package auth

import (
	"fmt"
	"net/http"
	"strings"

	authnv1 "k8s.io/api/authentication/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	authentication "k8s.io/client-go/kubernetes/typed/authentication/v1"
)

type Authenticator interface {
	AuthenticateRequest(req *http.Request) (AuthenticatedResult, error)
}

type AuthenticatedResult struct {
	Authenticated bool
	UserName      string
	Groups        []string
	Reason        string
}

func NewTokenAuthenticator(client authentication.TokenReviewInterface) Authenticator {
	return &tokenAuthenticator{
		client: client,
	}
}

type tokenAuthenticator struct {
	client authentication.TokenReviewInterface
}

func (t *tokenAuthenticator) AuthenticateRequest(req *http.Request) (AuthenticatedResult, error) {
	if req == nil {
		return AuthenticatedResult{}, fmt.Errorf("request is empty")
	}

	auth := req.Header.Get("Authorization")
	if auth == "" {
		return AuthenticatedResult{
			Reason: "authorization header is missing",
		}, nil
	}

	parts := strings.SplitN(auth, " ", 2)
	if len(parts) != 2 || parts[0] != "Bearer" {
		return AuthenticatedResult{
			Reason: "invalid authorization header format",
		}, nil
	}
	token := parts[1]

	tr := &authnv1.TokenReview{
		Spec: authnv1.TokenReviewSpec{
			Token: token,
		},
	}

	result, err := t.client.Create(req.Context(), tr, metav1.CreateOptions{})
	if err != nil {
		return AuthenticatedResult{
			Reason: "internal server error",
		}, err
	}

	return AuthenticatedResult{
		Authenticated: result.Status.Authenticated,
		UserName:      result.Status.User.Username,
		Groups:        result.Status.User.Groups,
	}, nil
}
