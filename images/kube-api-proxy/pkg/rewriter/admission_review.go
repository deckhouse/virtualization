package rewriter

import (
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

// RewriteAdmissionReview rewrites AdmissionReview request and response.
// NOTE: action is not supported yet, Restore is assumed for AdmissionReview
// request from Kubernetes API Server. Response is passed back to server as is.
func RewriteAdmissionReview(rules *RewriteRules, obj []byte, origGroup string) ([]byte, error) {
	response := gjson.GetBytes(obj, "response")
	if response.Exists() {
		// TODO rewrite response with the Patch.
		return obj, nil
	}

	request := gjson.GetBytes(obj, "request")
	if request.Exists() {
		newRequest, err := RewriteAdmissionReviewRequest(rules, []byte(request.Raw), origGroup)
		if err != nil {
			return nil, err
		}
		if len(newRequest) > 0 {
			obj, err = sjson.SetRawBytes(obj, "request", newRequest)
			if err != nil {
				return nil, err
			}
		}
	}

	return obj, nil
}

// RewriteAdmissionReviewRequest restores apiVersion, kind and other fields in an AdmissionReview request.
// Only restoring is required, as AdmissionReview request only comes from API Server.
func RewriteAdmissionReviewRequest(rules *RewriteRules, obj []byte, origGroup string) ([]byte, error) {
	var err error

	// Rewrite "resource" field and find rules.
	{
		resourceObj := gjson.GetBytes(obj, "resource")
		group := resourceObj.Get("group")
		resource := resourceObj.Get("resource")
		// Ignore reviews for unknown renamed group.
		if group.String() != rules.RenamedGroup {
			return nil, nil
		}
		newResource := rules.RestoreResource(resource.String())
		obj, err = sjson.SetBytes(obj, "resource.resource", newResource)
		if err != nil {
			return nil, err
		}
		obj, err = sjson.SetBytes(obj, "resource.group", origGroup)
		if err != nil {
			return nil, err
		}
	}

	// Rewrite "requestResource" field.
	{
		fieldObj := gjson.GetBytes(obj, "requestResource")
		group := fieldObj.Get("group")
		resource := fieldObj.Get("resource")
		// Ignore reviews for unknown renamed group.
		if group.String() != rules.RenamedGroup {
			return nil, nil
		}
		newResource := rules.RestoreResource(resource.String())
		obj, err = sjson.SetBytes(obj, "requestResource.resource", newResource)
		if err != nil {
			return nil, err
		}
		obj, err = sjson.SetBytes(obj, "requestResource.group", origGroup)
		if err != nil {
			return nil, err
		}
	}

	// Check "subresource" field. No need to rewrite kind, requestKind, object and oldObject fields if subresource is set.
	{
		fieldObj := gjson.GetBytes(obj, "subresource")
		if fieldObj.Exists() && fieldObj.String() != "" {
			return obj, err
		}
	}

	// Rewrite "kind" field.
	{
		fieldObj := gjson.GetBytes(obj, "kind")
		kind := fieldObj.Get("kind")
		newKind := rules.RestoreKind(kind.String())
		obj, err = sjson.SetBytes(obj, "kind.kind", newKind)
		if err != nil {
			return nil, err
		}
		obj, err = sjson.SetBytes(obj, "kind.group", origGroup)
		if err != nil {
			return nil, err
		}
	}

	// Rewrite "requestKind" field.
	{
		fieldObj := gjson.GetBytes(obj, "requestKind")
		kind := fieldObj.Get("kind")
		newKind := rules.RestoreKind(kind.String())
		obj, err = sjson.SetBytes(obj, "requestKind.kind", newKind)
		if err != nil {
			return nil, err
		}
		obj, err = sjson.SetBytes(obj, "requestKind.group", origGroup)
		if err != nil {
			return nil, err
		}
	}

	// Rewrite "object" field.
	{
		fieldObj := gjson.GetBytes(obj, "object")
		if fieldObj.Exists() {
			newField, err := RestoreResource(rules, []byte(fieldObj.Raw), origGroup)
			if err != nil {
				return nil, err
			}
			if len(newField) > 0 {
				obj, err = sjson.SetRawBytes(obj, "object", newField)
				if err != nil {
					return nil, err
				}
			}
		}
	}

	// Rewrite "oldObject" field.
	{
		fieldObj := gjson.GetBytes(obj, "oldObject")
		if fieldObj.Exists() {
			newField, err := RestoreResource(rules, []byte(fieldObj.Raw), origGroup)
			if err != nil {
				return nil, err
			}
			if len(newField) > 0 {
				obj, err = sjson.SetRawBytes(obj, "oldObject", newField)
				if err != nil {
					return nil, err
				}
			}
		}
	}

	return obj, nil
}
