package executor

import (
	"bytes"
	"sync"
)

type Buffer struct {
	buf   bytes.Buffer
	mutex sync.RWMutex
}

func (b *Buffer) Write(in []byte) (int, error) {
	b.mutex.Lock()
	r, err := b.buf.Write(in)
	b.mutex.Unlock()
	return r, err
}

func (b *Buffer) String() string {
	b.mutex.Lock()
	r := b.buf.String()
	b.mutex.Unlock()
	return r
}

func (b *Buffer) Reset() {
	b.mutex.Lock()
	b.buf.Reset()
	b.mutex.Unlock()
}

func (b *Buffer) Len() int {
	b.mutex.Lock()
	r := b.buf.Len()
	b.mutex.Unlock()
	return r
}

func (b *Buffer) Bytes() []byte {
	b.mutex.Lock()
	r := b.buf.Bytes()
	b.mutex.Unlock()
	return r
}
