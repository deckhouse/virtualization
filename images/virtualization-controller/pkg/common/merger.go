package common

// MergeLabels merges maps of labels into one map.
// Labels in the first argument are
// overridden with labels from the next argument and so on.
func MergeLabels(in ...map[string]string) map[string]string {
	res := make(map[string]string)

	for _, labels := range in {
		for k, v := range labels {
			res[k] = v
		}
	}

	return res
}

// ApplyMapChanges merges to the target all keys and values from the current version,
// removes from the target the keys that were present in the previous version but are absent in the current one.
// It returns true if the keys or values of the target have changed.
func ApplyMapChanges(target, prev, cur map[string]string) (map[string]string, bool) {
	if target == nil {
		target = map[string]string{}
	}

	var isChanged bool

	for key, value := range cur {
		if target[key] != value {
			target[key] = value
			isChanged = true
		}
	}

	for key := range prev {
		_, currHasKey := cur[key]
		_, targetHasKey := target[key]
		if !currHasKey && targetHasKey {
			delete(target, key)
			isChanged = true
		}
	}

	return target, isChanged
}
