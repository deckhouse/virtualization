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

package pwgen

import "crypto/rand"

var (
	num            = "0123456789"
	lowercaseAlpha = "abcdefghijklmnopqrstuvwxyz"
	alpha          = "ABCDEFGHIJKLMNOPQRSTUVWXYZ" + lowercaseAlpha
	alphaNum       = num + alpha
)

func generateString(length int, chars string) string {
	bytes := make([]byte, length)
	op := byte(len(chars))

	_, _ = rand.Read(bytes)
	for i, b := range bytes {
		bytes[i] = chars[b%op]
	}
	return string(bytes)
}

func AlphaNum(length int) string {
	return generateString(length, alphaNum)
}

func LowerAlpha(length int) string {
	return generateString(length, lowercaseAlpha)
}
