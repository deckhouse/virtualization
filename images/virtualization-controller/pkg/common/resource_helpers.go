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
