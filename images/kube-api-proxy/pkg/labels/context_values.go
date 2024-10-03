package labels

import (
	"context"
	"strconv"
)

func ContextWithCommon(ctx context.Context, name, resource, method string) context.Context {
	ctx = context.WithValue(ctx, nameKey{}, name)
	ctx = context.WithValue(ctx, resourceKey{}, resource)
	return context.WithValue(ctx, methodKey{}, method)
}

func ContextWithDecision(ctx context.Context, decision string) context.Context {
	return context.WithValue(ctx, decisionKey{}, decision)
}

func ContextWithStatus(ctx context.Context, status int) context.Context {
	return context.WithValue(ctx, statusKey{}, strconv.Itoa(status))
}

type nameKey struct{}
type resourceKey struct{}
type methodKey struct{}
type decisionKey struct{}
type statusKey struct{}

func NameFromContext(ctx context.Context) string {
	if method, ok := ctx.Value(nameKey{}).(string); ok {
		return method
	}
	return ""
}

func ResourceFromContext(ctx context.Context) string {
	if method, ok := ctx.Value(resourceKey{}).(string); ok {
		return method
	}
	return ""
}

func MethodFromContext(ctx context.Context) string {
	if method, ok := ctx.Value(methodKey{}).(string); ok {
		return method
	}
	return ""
}

func DecisionFromContext(ctx context.Context) string {
	if decision, ok := ctx.Value(decisionKey{}).(string); ok {
		return decision
	}
	return ""
}

func StatusFromContext(ctx context.Context) string {
	if decision, ok := ctx.Value(statusKey{}).(string); ok {
		return decision
	}
	return ""
}
