package rewriter

const (
	PodDisruptionBudgetKind     = "PodDisruptionBudget"
	PodDisruptionBudgetListKind = "PodDisruptionBudgetList"
)

func RewritePDBOrList(rules *RewriteRules, obj []byte, action Action) ([]byte, error) {
	if action == Rename {
		return RewriteResourceOrList(obj, PodDisruptionBudgetListKind, func(singleObj []byte) ([]byte, error) {
			return RewriteMapOfStrings(singleObj, "spec.selector", rules.RenameLabels)
		})
	}
	return RewriteResourceOrList(obj, PodDisruptionBudgetListKind, func(singleObj []byte) ([]byte, error) {
		return RewriteMapOfStrings(singleObj, "spec.selector", rules.RestoreLabels)
	})
}
