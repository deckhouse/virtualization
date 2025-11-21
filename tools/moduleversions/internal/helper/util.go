package helper

import "strings"

func GetSemVer(version string) string {
	if strings.HasPrefix(version, "v") {
		version = strings.TrimPrefix(version, "v")
		return version
	} else {
		return version
	}
}

func GetChannel(channel string) string {
	channel = strings.ReplaceAll(strings.ToLower(channel), " ", "-")
	return channel
}
