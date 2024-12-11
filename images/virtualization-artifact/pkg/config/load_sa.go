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

package config

import (
	"fmt"
	"os"
)

const (
	SACDIApiserver                = "SA_CDI_APISERVER"
	SACDICronjob                  = "SA_CDI_CRONJOB"
	SACDIOperator                 = "SA_CDI_OPERATOR"
	SACDISa                       = "SA_CDI_SA"
	SACDIUploadProxy              = "SA_CDI_UPLOAD_PROXY"
	SAKubevirtApiserver           = "SA_KUBEVIRT_APISERVER"
	SAKubevirtController          = "SA_KUBEVIRT_CONTROLLER"
	SAKubevirtExportProxy         = "SA_KUBEVIRT_EXPORT_PROXY"
	SAKubevirtVirtHandler         = "SA_KUBEVIRT_VIRT_HANDLER"
	SAKubevirtOperator            = "SA_KUBEVIRT_OPERATOR"
	SAVirtualizationController    = "SA_VIRTUALIZATION_CONTROLLER"
	SAVirtualizationAPI           = "SA_VIRTUALIZATION_API"
	SAVirtualizationPpeDeleteHook = "SA_VIRTUALIZATION_PPE_DELETE_HOOK"
	SAVmRouteForge                = "SA_VM_ROUTE_FORGE"
)

type ServiceAccounts struct {
	SACDIApiserver                string
	SACDICronjob                  string
	SACDIOperator                 string
	SACDISa                       string
	SACDIUploadProxy              string
	SAKubevirtApiserver           string
	SAKubevirtController          string
	SAKubevirtExportProxy         string
	SAKubevirtVirtHandler         string
	SAKubevirtOperator            string
	SAVirtualizationController    string
	SAVirtualizationAPI           string
	SAVirtualizationPpeDeleteHook string
	SAVmRouteForge                string
}

func (s *ServiceAccounts) ToList() []string {
	return []string{
		s.SACDIApiserver,
		s.SACDICronjob,
		s.SACDIOperator,
		s.SACDISa,
		s.SACDIUploadProxy,
		s.SAKubevirtApiserver,
		s.SAKubevirtController,
		s.SAKubevirtExportProxy,
		s.SAKubevirtVirtHandler,
		s.SAKubevirtOperator,
		s.SAVirtualizationController,
		s.SAVirtualizationAPI,
		s.SAVirtualizationPpeDeleteHook,
		s.SAVmRouteForge,
	}
}

func (s *ServiceAccounts) Validate() error {
	if s.SACDIApiserver == "" {
		return fmt.Errorf("%q is required", SACDIApiserver)
	}
	if s.SACDICronjob == "" {
		return fmt.Errorf("%q is required", SACDICronjob)
	}
	if s.SACDIOperator == "" {
		return fmt.Errorf("%q is required", SACDIOperator)
	}
	if s.SACDISa == "" {
		return fmt.Errorf("%q is required", SACDISa)
	}
	if s.SACDIUploadProxy == "" {
		return fmt.Errorf("%q is required", SACDIUploadProxy)
	}
	if s.SAKubevirtApiserver == "" {
		return fmt.Errorf("%q is required", SAKubevirtApiserver)
	}
	if s.SAKubevirtController == "" {
		return fmt.Errorf("%q is required", SAKubevirtController)
	}
	if s.SAKubevirtExportProxy == "" {
		return fmt.Errorf("%q is required", SAKubevirtExportProxy)
	}
	if s.SAKubevirtVirtHandler == "" {
		return fmt.Errorf("%q is required", SAKubevirtVirtHandler)
	}
	if s.SAKubevirtOperator == "" {
		return fmt.Errorf("%q is required", SAKubevirtOperator)
	}
	if s.SAVirtualizationController == "" {
		return fmt.Errorf("%q is required", SAVirtualizationController)
	}
	if s.SAVirtualizationAPI == "" {
		return fmt.Errorf("%q is required", SAVirtualizationAPI)
	}
	if s.SAVirtualizationPpeDeleteHook == "" {
		return fmt.Errorf("%q is required", SAVirtualizationPpeDeleteHook)
	}
	if s.SAVmRouteForge == "" {
		return fmt.Errorf("%q is required", SAVmRouteForge)
	}
	return nil
}

func LoadServiceAccounts() (ServiceAccounts, error) {
	serviceAccounts := ServiceAccounts{
		SACDIApiserver:                os.Getenv(SACDIApiserver),
		SACDICronjob:                  os.Getenv(SACDICronjob),
		SACDIOperator:                 os.Getenv(SACDIOperator),
		SACDISa:                       os.Getenv(SACDISa),
		SACDIUploadProxy:              os.Getenv(SACDIUploadProxy),
		SAKubevirtApiserver:           os.Getenv(SAKubevirtApiserver),
		SAKubevirtController:          os.Getenv(SAKubevirtController),
		SAKubevirtExportProxy:         os.Getenv(SAKubevirtExportProxy),
		SAKubevirtVirtHandler:         os.Getenv(SAKubevirtVirtHandler),
		SAKubevirtOperator:            os.Getenv(SAKubevirtOperator),
		SAVirtualizationController:    os.Getenv(SAVirtualizationController),
		SAVirtualizationAPI:           os.Getenv(SAVirtualizationAPI),
		SAVirtualizationPpeDeleteHook: os.Getenv(SAVirtualizationPpeDeleteHook),
		SAVmRouteForge:                os.Getenv(SAVmRouteForge),
	}
	if err := serviceAccounts.Validate(); err != nil {
		return ServiceAccounts{}, err
	}
	return serviceAccounts, nil
}
