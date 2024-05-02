package rewriter

const (
	ValidatingWebhookConfigurationKind     = "ValidatingWebhookConfiguration"
	ValidatingWebhookConfigurationListKind = "ValidatingWebhookConfigurationList"
	MutatingWebhookConfigurationKind       = "MutatingWebhookConfiguration"
	MutatingWebhookConfigurationListKind   = "MutatingWebhookConfigurationList"
)

func RewriteValidatingOrList(rules *RewriteRules, obj []byte, action Action) ([]byte, error) {
	if action == Rename {
		return RewriteResourceOrList(obj, ValidatingWebhookConfigurationListKind, func(singleObj []byte) ([]byte, error) {
			return RewriteArray(singleObj, "webhooks", func(webhook []byte) ([]byte, error) {
				return RewriteArray(webhook, "rules", func(item []byte) ([]byte, error) {
					return renameRoleRule(rules, item)
				})
			})
		})
	}
	return RewriteResourceOrList(obj, ValidatingWebhookConfigurationListKind, func(singleObj []byte) ([]byte, error) {
		return RewriteArray(singleObj, "webhooks", func(webhook []byte) ([]byte, error) {
			return RewriteArray(webhook, "rules", func(item []byte) ([]byte, error) {
				return restoreRoleRule(rules, item)
			})
		})
	})
}

func RewriteMutatingOrList(rules *RewriteRules, obj []byte, action Action) ([]byte, error) {
	if action == Rename {
		return RewriteResourceOrList(obj, MutatingWebhookConfigurationListKind, func(singleObj []byte) ([]byte, error) {
			return RewriteArray(singleObj, "webhooks", func(webhook []byte) ([]byte, error) {
				return RewriteArray(webhook, "rules", func(item []byte) ([]byte, error) {
					return renameRoleRule(rules, item)
				})
			})
		})
	}
	return RewriteResourceOrList(obj, MutatingWebhookConfigurationListKind, func(singleObj []byte) ([]byte, error) {
		return RewriteArray(singleObj, "webhooks", func(webhook []byte) ([]byte, error) {
			return RewriteArray(webhook, "rules", func(item []byte) ([]byte, error) {
				return restoreRoleRule(rules, item)
			})
		})
	})
}
