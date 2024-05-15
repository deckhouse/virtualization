package rewriter

const (
	DeploymentKind      = "Deployment"
	DeploymentListKind  = "DeploymentList"
	DaemonSetKind       = "DaemonSet"
	DaemonSetListKind   = "DaemonSetList"
	StatefulSetKind     = "StatefulSet"
	StatefulSetListKind = "StatefulSetList"
)

func RewriteDeploymentOrList(rules *RewriteRules, obj []byte, action Action) ([]byte, error) {
	if action == Rename {
		return RewriteResourceOrList(obj, DeploymentListKind, renameSpecLabelsAnno(rules))
	}
	return RewriteResourceOrList(obj, DeploymentListKind, restoreSpecLabelsAnno(rules))
}

func RewriteDaemonSetOrList(rules *RewriteRules, obj []byte, action Action) ([]byte, error) {
	if action == Rename {
		return RewriteResourceOrList(obj, DaemonSetListKind, renameSpecLabelsAnno(rules))
	}
	return RewriteResourceOrList(obj, DaemonSetListKind, restoreSpecLabelsAnno(rules))
}

func RewriteStatefulSetOrList(rules *RewriteRules, obj []byte, action Action) ([]byte, error) {
	if action == Rename {
		return RewriteResourceOrList(obj, StatefulSetListKind, renameSpecLabelsAnno(rules))
	}
	return RewriteResourceOrList(obj, StatefulSetListKind, restoreSpecLabelsAnno(rules))
}

func renameSpecLabelsAnno(rules *RewriteRules) func(singleObj []byte) ([]byte, error) {
	return func(singleObj []byte) ([]byte, error) {
		singleObj, err := RewriteMapOfStrings(singleObj, "spec.template.metadata.labels", rules.RenameLabels)
		if err != nil {
			return nil, err
		}
		singleObj, err = RewriteMapOfStrings(singleObj, "spec.selector.matchLabels", rules.RenameLabels)
		if err != nil {
			return nil, err
		}
		singleObj, err = RewriteMapOfStrings(singleObj, "spec.template.spec.nodeSelector", rules.RenameLabels)
		if err != nil {
			return nil, err
		}
		return RewriteMapOfStrings(singleObj, "spec.template.metadata.annotations", rules.RenameAnnotations)
	}
}

func restoreSpecLabelsAnno(rules *RewriteRules) func(singleObj []byte) ([]byte, error) {
	return func(singleObj []byte) ([]byte, error) {
		singleObj, err := RewriteMapOfStrings(singleObj, "spec.template.metadata.labels", rules.RestoreLabels)
		if err != nil {
			return nil, err
		}
		singleObj, err = RewriteMapOfStrings(singleObj, "spec.selector.matchLabels", rules.RestoreLabels)
		if err != nil {
			return nil, err
		}
		singleObj, err = RewriteMapOfStrings(singleObj, "spec.template.spec.nodeSelector", rules.RestoreLabels)
		if err != nil {
			return nil, err
		}
		return RewriteMapOfStrings(singleObj, "spec.template.metadata.annotations", rules.RestoreAnnotations)
	}
}
