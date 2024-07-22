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

package service

import "errors"

var (
	ErrStorageClassNotFound        = errors.New("storage class not found")
	ErrDefaultStorageClassNotFound = errors.New("default storage class not found")
	ErrDataVolumeNotRunning        = errors.New("pvc import is not running")
)

var (
	ErrInvalidIpAddress      = errors.New("Invalid IP address format")
	ErrIpAddressAlreadyExist = errors.New("IP address is already allocated")
	ErrIpAddressOutOfRange   = errors.New("IP address is out of range")
)
