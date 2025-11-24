package helper

import "strings"

func GetSemVer(version string) string {
	return strings.TrimPrefix(version, "v")
}

func GetChannel(channel string) string {
	channel = strings.ReplaceAll(strings.ToLower(channel), " ", "-")
	return channel
}
