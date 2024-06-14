/*
Copyright 2024 Flant JSC

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

     http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

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
