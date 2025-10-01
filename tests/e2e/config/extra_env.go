/*
Copyright 2025 Flant JSC

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

package config

const (
	// E2EVolumeMigrationNextStorageClassEnv is the env variable for the next storage class for volume migration tests.
	E2EVolumeMigrationNextStorageClassEnv = "E2E_VOLUME_MIGRATION_NEXT_STORAGE_CLASS"
)

const (
	E2EShortTimeoutEnv  = "E2E_SHORT_TIMEOUT"
	E2EMiddleTimeoutEnv = "E2E_MIDDLE_TIMEOUT"
	E2ELongTimeoutEnv   = "E2E_LONG_TIMEOUT"
	E2EMaxTimeoutEnv    = "E2E_MAX_TIMEOUT"
)
