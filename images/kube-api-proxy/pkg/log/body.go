package log

import (
	"bytes"
	"fmt"
	"io"
)

type ReaderLogger struct {
	wrappedReader io.ReadCloser
	buf           bytes.Buffer
}

func NewReaderLogger(r io.Reader) *ReaderLogger {
	rdr := &ReaderLogger{}
	rdr.wrappedReader = io.NopCloser(io.TeeReader(r, &rdr.buf))
	return rdr
}

func (r *ReaderLogger) Read(p []byte) (n int, err error) {
	return r.wrappedReader.Read(p)
}

func (r *ReaderLogger) Close() error {
	return r.wrappedReader.Close()
}

func HeadString(obj interface{}, limit int) string {
	readLog, ok := obj.(*ReaderLogger)
	if !ok {
		return ""
	}
	bufLen := readLog.buf.Len()
	bufStr := readLog.buf.String()
	if bufLen < limit {
		return bufStr
	}
	return bufStr[0:limit]
}

func HeadStringEx(obj interface{}, limit int) string {
	s := HeadString(obj, limit)
	if s == "" {
		return "<empty>"
	}
	return fmt.Sprintf("[%d] %s", len(s), s)
}

func HasData(obj interface{}) bool {
	readLog, ok := obj.(*ReaderLogger)
	if !ok {
		return false
	}
	return readLog.buf.Len() > 0
}
