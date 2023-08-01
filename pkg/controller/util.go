package controller

func HasLabel(labels map[string]string, matchFunc func(k, v string) bool) bool {
	for k, v := range labels {
		if matchFunc(k, v) {
			return true
		}
	}
	return false
}
