package log

import "log/slog"

func SlogErr(err error) slog.Attr {
	return slog.Any("err", err)
}
