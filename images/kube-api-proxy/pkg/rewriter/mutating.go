package rewriter

const (
	MutatingWebhookConfigurationKind     = "MutatingWebhookConfiguration"
	MutatingWebhookConfigurationListKind = "MutatingWebhookConfigurationList"
)

func RewriteMutatingOrList(rules *RewriteRules, obj []byte, action Action) ([]byte, error) {
	return obj, nil
}
