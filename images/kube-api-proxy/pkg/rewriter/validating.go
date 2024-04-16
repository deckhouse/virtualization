package rewriter

const (
	ValidatingWebhookConfigurationKind     = "ValidatingWebhookConfiguration"
	ValidatingWebhookConfigurationListKind = "ValidatingWebhookConfigurationList"
)

func RewriteValidatingOrList(rules *RewriteRules, obj []byte, action Action) ([]byte, error) {
	return obj, nil
}
