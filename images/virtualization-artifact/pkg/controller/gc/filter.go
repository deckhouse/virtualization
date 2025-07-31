package gc

import (
	"cmp"
	"slices"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/deckhouse/virtualization-controller/pkg/common/object"
)

type IsCandidate func(obj client.Object) bool
type IndexFunc func(obj client.Object) string

func DefaultFilter(objs []client.Object, isCandidate IsCandidate, ttl time.Duration, indexFunc IndexFunc, maxCount int) []client.Object {
	var (
		expired    []client.Object
		nonExpired []client.Object
	)

	for _, obj := range objs {
		isCandidate := isCandidate(obj)
		if !isCandidate {
			continue
		}

		if object.IsTerminating(obj) {
			continue
		}

		if object.GetAge(obj) > ttl {
			expired = append(expired, obj)
			continue
		}

		nonExpired = append(nonExpired, obj)
	}

	result := expired
	if maxCount <= 0 {
		return result
	}

	slices.SortFunc(nonExpired, func(a, b client.Object) int {
		return cmp.Compare(object.GetAge(a), object.GetAge(b))
	})

	indexed := make(map[string]int)
	for _, obj := range nonExpired {
		index := indexFunc(obj)
		count := indexed[index]
		if count >= maxCount {
			result = append(result, obj)
		}
		indexed[index]++
	}

	return result
}
