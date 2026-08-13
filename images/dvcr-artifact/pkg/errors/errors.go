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

package errors

import "fmt"

type ReasonError interface {
	error
	Reason() string
}

func NewBadImageChecksumError(expectedSum, actualSum, algorithm string) BadImageChecksumError {
	return BadImageChecksumError{
		algorithm: algorithm,
		expected:  expectedSum,
		actual:    actualSum,
	}
}

type BadImageChecksumError struct {
	expected  string
	actual    string
	algorithm string
}

func (e BadImageChecksumError) Reason() string {
	return "BadImageChecksum"
}

// Permanent reports that repeating the import cannot fix the mismatch: the
// source has already been read to the end, and downloading the very same image
// again can only produce the very same sum. Retrying it would hide the typo in
// the checksum behind the whole backoff, re-downloading the image on every
// attempt.
func (e BadImageChecksumError) Permanent() bool {
	return true
}

func (e BadImageChecksumError) Error() string {
	return fmt.Sprintf("%s sum mismatch: %s != %s", e.algorithm, e.expected, e.actual)
}
