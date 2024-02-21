package util

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
