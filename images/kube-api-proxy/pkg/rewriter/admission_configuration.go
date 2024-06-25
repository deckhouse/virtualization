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

package rewriter

import (
	"github.com/tidwall/gjson"

	"kube-api-proxy/pkg/rewriter/rules"
	"kube-api-proxy/pkg/rewriter/transform"
)

const (
	ValidatingWebhookConfigurationKind     = "ValidatingWebhookConfiguration"
	ValidatingWebhookConfigurationListKind = "ValidatingWebhookConfigurationList"
	MutatingWebhookConfigurationKind       = "MutatingWebhookConfiguration"
	MutatingWebhookConfigurationListKind   = "MutatingWebhookConfigurationList"
)

func RewriteValidating(rwRules *rules.RewriteRules, validatingObj []byte, action rules.Action) ([]byte, error) {
	return transform.Array(validatingObj, "webhooks", func(webhookObj []byte) ([]byte, error) {
		return transform.Array(webhookObj, "rules", func(ruleObj []byte) ([]byte, error) {
			return RewriteRoleRule(rwRules, ruleObj, action)
		})
	})
}

func RewriteMutating(rwRules *rules.RewriteRules, mutatingObj []byte, action rules.Action) ([]byte, error) {
	return transform.Array(mutatingObj, "webhooks", func(webhookObj []byte) ([]byte, error) {
		return transform.Array(webhookObj, "rules", func(ruleObj []byte) ([]byte, error) {
			return RewriteRoleRule(rwRules, ruleObj, action)
		})
	})
}

func RenameWebhookConfigurationPatch(rwRules *rules.RewriteRules, obj []byte) ([]byte, error) {
	obj, err := RenameMetadataPatch(rwRules, obj)
	if err != nil {
		return nil, err
	}

	return transform.Patch(obj, func(mergePatch []byte) ([]byte, error) {
		return transform.Array(mergePatch, "webhooks", func(webhook []byte) ([]byte, error) {
			return transform.Array(webhook, "rules", func(ruleObj []byte) ([]byte, error) {
				return RewriteRoleRule(rwRules, ruleObj, rules.Rename)
			})
		})
	}, func(jsonPatch []byte) ([]byte, error) {
		path := gjson.GetBytes(jsonPatch, "path").String()
		if path == "/webhooks" {
			return transform.Array(jsonPatch, "value", func(webhook []byte) ([]byte, error) {
				return transform.Array(webhook, "rules", func(ruleObj []byte) ([]byte, error) {
					return RewriteRoleRule(rwRules, ruleObj, rules.Rename)
				})
			})
		}
		return jsonPatch, nil
	})
}
