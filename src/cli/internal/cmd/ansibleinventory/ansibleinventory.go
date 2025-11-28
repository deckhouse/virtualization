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

package ansibleinventory

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/src/cli/internal/clientconfig"
	"github.com/deckhouse/virtualization/src/cli/internal/templates"
)

type AnsibleInventory struct {
	options Options
}

type Options struct {
	List bool
	Host string
}

func DefaultOptions() Options {
	return Options{
		List: false,
		Host: "",
	}
}

func NewCommand() *cobra.Command {
	c := &AnsibleInventory{
		options: DefaultOptions(),
	}

	cmd := &cobra.Command{
		Use:   "ansible-inventory",
		Short: "Generate ansible inventory from virtual machines (ansible inventory script compatible).",
		Long: `Generate ansible inventory from virtual machines.
This command is compatible with ansible inventory script interface.

Required arguments (one of):
  --list              Return all hosts in JSON format
  --host <hostname>   Return variables for specific host in JSON format`,
		Example: usage(),
		Args:    cobra.NoArgs,
		RunE:    c.Run,
	}

	AddCommandlineArgs(cmd.Flags(), &c.options)
	cmd.SetUsageTemplate(templates.UsageTemplate())
	return cmd
}

func AddCommandlineArgs(flagset *pflag.FlagSet, opts *Options) {
	flagset.BoolVar(&opts.List, "list", opts.List,
		"Return all hosts in JSON format (required if --host is not specified)")
	flagset.StringVar(&opts.Host, "host", opts.Host,
		"Return variables for specific host in JSON format (required if --list is not specified)")
}

func (a *AnsibleInventory) Run(cmd *cobra.Command, args []string) error {
	if !a.options.List && a.options.Host == "" {
		return fmt.Errorf("one of --list or --host must be specified")
	}

	if a.options.List && a.options.Host != "" {
		return fmt.Errorf("--list and --host are mutually exclusive")
	}

	client, namespace, _, err := clientconfig.ClientAndNamespaceFromContext(cmd.Context())
	if err != nil {
		return err
	}

	vmList, err := client.VirtualMachines(namespace).List(context.TODO(), metav1.ListOptions{})
	if err != nil {
		return fmt.Errorf("failed to list virtual machines: %w", err)
	}
	allVMs := vmList.Items

	if a.options.Host != "" {
		hostInfo := a.generateHostInfo(allVMs, a.options.Host)
		if hostInfo == "" {
			cmd.Print("{}")
			return nil
		}
		cmd.Print(hostInfo)
		return nil
	}

	if a.options.List {
		inventory := a.generateInventoryJSON(allVMs)
		cmd.Print(inventory)
		return nil
	}

	return nil
}

func (a *AnsibleInventory) generateInventoryJSON(vms []v1alpha2.VirtualMachine) string {
	inventory := map[string]interface{}{
		"_meta": map[string]interface{}{
			"hostvars": make(map[string]interface{}),
		},
		"all": map[string]interface{}{
			"hosts": []string{},
			"vars": map[string]interface{}{
				"ansible_ssh_common_args": `-o ProxyCommand="d8 v port-forward --stdio=true %h %p"`,
			},
		},
	}

	for _, vm := range vms {
		// Skip VMs without IP address
		if vm.Status.IPAddress == "" {
			continue
		}

		hostName := a.getHostName(vm)
		allHosts := inventory["all"].(map[string]interface{})["hosts"].([]string)
		inventory["all"].(map[string]interface{})["hosts"] = append(allHosts, hostName)

		hostVars := a.getHostVars(vm)
		inventory["_meta"].(map[string]interface{})["hostvars"].(map[string]interface{})[hostName] = hostVars

		a.addVMToGroups(inventory, vm, hostName)
	}

	output, err := json.MarshalIndent(inventory, "", "  ")
	if err != nil {
		return fmt.Sprintf("Error: %v", err)
	}

	return string(output)
}

func (a *AnsibleInventory) generateHostInfo(vms []v1alpha2.VirtualMachine, hostName string) string {
	for _, vm := range vms {
		vmHostName := a.getHostName(vm)
		// Support search by both full name (namespace.vmname) and short name (vmname)
		if (vmHostName == hostName || vm.Name == hostName) && vm.Status.IPAddress != "" {
			hostVars := a.getHostVars(vm)

			output, err := json.MarshalIndent(hostVars, "", "  ")
			if err != nil {
				return ""
			}

			return string(output)
		}
	}
	return ""
}

func (a *AnsibleInventory) getHostName(vm v1alpha2.VirtualMachine) string {
	return fmt.Sprintf("%s.%s", vm.Namespace, vm.Name)
}

func (a *AnsibleInventory) getHostVars(vm v1alpha2.VirtualMachine) map[string]interface{} {
	hostVars := map[string]interface{}{}

	// Add annotations as host variables
	// Only process annotations with prefix provisioning.virtualization.deckhouse.io/
	const annotationPrefix = "provisioning.virtualization.deckhouse.io/"
	const groupsAnnotationKey = annotationPrefix + "groups"
	if len(vm.Annotations) > 0 {
		for key, value := range vm.Annotations {
			// Only process annotations with the specific prefix
			if !strings.HasPrefix(key, annotationPrefix) {
				continue
			}
			if key == groupsAnnotationKey {
				continue
			}
			varName := strings.TrimPrefix(key, annotationPrefix)
			if varName != "" {
				hostVars[varName] = value
			}
		}
	}

	return hostVars
}

func (a *AnsibleInventory) addVMToGroups(inventory map[string]interface{}, vm v1alpha2.VirtualMachine, hostName string) {
	const groupsAnnotationKey = "provisioning.virtualization.deckhouse.io/groups"

	if vm.Annotations == nil {
		return
	}

	groupsValue, exists := vm.Annotations[groupsAnnotationKey]
	if !exists || groupsValue == "" {
		return
	}

	groups := strings.Split(groupsValue, ",")
	for _, group := range groups {
		group = strings.TrimSpace(group)
		if group == "" {
			continue
		}

		if _, exists := inventory[group]; !exists {
			inventory[group] = map[string]interface{}{
				"hosts": []string{},
			}
		}

		groupHosts := inventory[group].(map[string]interface{})["hosts"].([]string)
		inventory[group].(map[string]interface{})["hosts"] = append(groupHosts, hostName)
	}
}

func usage() string {
	return `  # Standard ansible inventory script interface:
  # Return all hosts in JSON format (required):
  {{ProgramName}} ansible-inventory --list

  # Return variables for specific host in JSON format (required):
  {{ProgramName}} ansible-inventory --host <hostname>
  {{ProgramName}} ansible-inventory --host default.vm1

  # Specify namespace:
  {{ProgramName}} ansible-inventory --list -n mynamespace
  {{ProgramName}} ansible-inventory --host vm1 -n mynamespace

  # Host names format: namespace.vmname (e.g., default.vm1)
  # VM annotations with prefix provisioning.virtualization.deckhouse.io/ are included as host variables
  # Use provisioning.virtualization.deckhouse.io/groups annotation to add VMs to groups`
}
