package log

import "log/slog"

func SlogErr(err error) slog.Attr {
	return slog.Any("err", err)
}

func BodyDiff(diff string) slog.Attr {
	return slog.String(BodyDiffKey, diff)
}

func BodyDump(dump string) slog.Attr {
	return slog.String(BodyDumpKey, dump)
}
