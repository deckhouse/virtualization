package events

type ttlCache interface {
	Get(key string) (any, bool)
}

type indexer interface {
	GetByKey(string) (any, bool, error)
}
