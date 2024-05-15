package rewriter

const (
	PodKind     = "Pod"
	PodListKind = "PodList"
)

func RewritePodOrList(rules *RewriteRules, obj []byte, action Action) ([]byte, error) {
	if action == Rename {
		return RewriteResourceOrList(obj, PodListKind, func(singleObj []byte) ([]byte, error) {
			return RewriteMapOfStrings(singleObj, "spec.nodeSelector", rules.RenameLabels)
		})
	}
	return RewriteResourceOrList(obj, PodListKind, func(singleObj []byte) ([]byte, error) {
		return RewriteMapOfStrings(singleObj, "spec.nodeSelector", rules.RestoreLabels)
	})
}
