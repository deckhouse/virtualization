/*
Copyright 2026 Flant JSC

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

package registry

type TailBuffer struct {
	buffer   []byte
	size     int
	writePos int
	full     bool
}

func NewTailBuffer(size int) *TailBuffer {
	return &TailBuffer{
		buffer: make([]byte, size),
		size:   size,
	}
}

func (tb *TailBuffer) Write(p []byte) (n int, err error) {
	n = len(p)

	for len(p) > 0 {
		available := tb.size - tb.writePos
		toCopy := len(p)
		if toCopy > available {
			toCopy = available
		}

		copy(tb.buffer[tb.writePos:], p[:toCopy])
		tb.writePos += toCopy
		p = p[toCopy:]

		if tb.writePos >= tb.size {
			tb.writePos = 0
			tb.full = true
		}
	}

	return n, nil
}

func (tb *TailBuffer) Bytes() []byte {
	if !tb.full {
		return tb.buffer[:tb.writePos]
	}

	result := make([]byte, tb.size)
	copy(result, tb.buffer[tb.writePos:])
	copy(result[tb.size-tb.writePos:], tb.buffer[:tb.writePos])
	return result
}
