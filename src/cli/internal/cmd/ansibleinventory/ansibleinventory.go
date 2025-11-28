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
	"gopkg.in/yaml.v3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/deckhouse/virtualization/api/core/v1alpha2"
	"github.com/deckhouse/virtualization/src/cli/internal/clientconfig"
	"github.com/deckhouse/virtualization/src/cli/internal/templates"
)

const (
	annotationPrefix        = "provisioning.virtualization.deckhouse.io/"
	groupsAnnotationKey     = annotationPrefix + "groups"
	ansibleSSHCommonArgs    = `-o ProxyCommand='d8 v port-forward --stdio=true %h %p'`
	ansibleSSHCommonArgsKey = "ansible_ssh_common_args"
)

type AnsibleInventory struct {
	options Options
}

type Options struct {
	List      bool
	Host      string
	Output    string
	Namespace string
}

func DefaultOptions() Options {
	return Options{
		List:      false,
		Host:      "",
		Output:    "yaml",
		Namespace: "",
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

When called without arguments, returns all hosts (same as --list).

Arguments:
  --list                   Return all hosts (default behavior if no arguments)
  --host <hostname>        Return variables for specific host
  --output, -o <format>    Output format: json, ini, or yaml (default: yaml)
  --namespace, -n <ns>     Namespace to list virtual machines from (overrides kubeconfig context namespace)

Host names format: <vmname>.<namespace> (e.g., myvm.default)
VM annotations with prefix provisioning.virtualization.deckhouse.io/ are included as host variables.
Use provisioning.virtualization.deckhouse.io/groups annotation to add VMs to groups.`,
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
		"Return all hosts (default behavior if no arguments)")
	flagset.StringVar(&opts.Host, "host", opts.Host,
		"Return variables for specific host")
	flagset.StringVarP(&opts.Output, "output", "o", opts.Output,
		"Output format: json, ini, or yaml (default: yaml)")
	flagset.StringVarP(&opts.Namespace, "namespace", "n", opts.Namespace,
		"Namespace to list virtual machines from (overrides kubeconfig context namespace)")
}

func (a *AnsibleInventory) Run(cmd *cobra.Command, args []string) error {
	if a.options.List && a.options.Host != "" {
		return fmt.Errorf("--list and --host are mutually exclusive")
	}

	if !a.options.List && a.options.Host == "" {
		a.options.List = true
	}

	client, defaultNamespace, _, err := clientconfig.ClientAndNamespaceFromContext(cmd.Context())
	if err != nil {
		return err
	}

	namespace := defaultNamespace
	if a.options.Namespace != "" {
		namespace = a.options.Namespace
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
		inventory := a.generateInventory(allVMs)
		cmd.Print(inventory)
		return nil
	}

	return nil
}

func (a *AnsibleInventory) generateInventory(vms []v1alpha2.VirtualMachine) string {
	switch strings.ToLower(a.options.Output) {
	case "ini":
		return a.generateInventoryINI(vms)
	case "yaml":
		return a.generateInventoryYAML(vms)
	default:
		return a.generateInventoryJSON(vms)
	}
}

func (a *AnsibleInventory) generateInventoryJSON(vms []v1alpha2.VirtualMachine) string {
	inventory := a.buildInventoryStructure(vms)

	output, err := json.MarshalIndent(inventory, "", "  ")
	if err != nil {
		return fmt.Sprintf("Error: %v", err)
	}

	return string(output)
}

func (a *AnsibleInventory) generateInventoryINI(vms []v1alpha2.VirtualMachine) string {
	var builder strings.Builder

	builder.WriteString("[all]\n")
	for _, vm := range vms {
		if vm.Status.IPAddress == "" {
			continue
		}
		hostName := a.getHostName(vm)
		builder.WriteString(fmt.Sprintf("%s\n", hostName))
	}

	builder.WriteString("\n[all:vars]\n")
	builder.WriteString(fmt.Sprintf("%s=\"%s\"\n", ansibleSSHCommonArgsKey, ansibleSSHCommonArgs))

	// Collect groups from annotations
	groupsMap := make(map[string][]string)
	for _, vm := range vms {
		if vm.Status.IPAddress == "" {
			continue
		}
		hostName := a.getHostName(vm)
		groups := a.getVMGroups(vm)
		for _, group := range groups {
			groupsMap[group] = append(groupsMap[group], hostName)
		}
	}

	// Write groups
	for group, hosts := range groupsMap {
		builder.WriteString(fmt.Sprintf("\n[%s]\n", group))
		for _, host := range hosts {
			builder.WriteString(fmt.Sprintf("%s\n", host))
		}
	}

	return builder.String()
}

func (a *AnsibleInventory) generateInventoryYAML(vms []v1alpha2.VirtualMachine) string {
	inventory := a.buildStaticYAMLInventory(vms)

	output, err := yaml.Marshal(inventory)
	if err != nil {
		return fmt.Sprintf("Error: %v", err)
	}

	return string(output)
}

func (a *AnsibleInventory) buildStaticYAMLInventory(vms []v1alpha2.VirtualMachine) map[string]interface{} {
	// For static YAML inventory, hosts should be a dictionary, not an array
	allHosts := make(map[string]interface{})
	groupsMap := make(map[string]map[string]interface{})

	for _, vm := range vms {
		if vm.Status.IPAddress == "" {
			continue
		}

		hostName := a.getHostName(vm)
		hostVars := a.getHostVars(vm)

		if len(hostVars) > 0 {
			allHosts[hostName] = hostVars
		} else {
			allHosts[hostName] = make(map[string]interface{})
		}

		groups := a.getVMGroups(vm)
		for _, group := range groups {
			if _, exists := groupsMap[group]; !exists {
				groupsMap[group] = make(map[string]interface{})
			}
			if len(hostVars) > 0 {
				groupsMap[group][hostName] = hostVars
			} else {
				groupsMap[group][hostName] = make(map[string]interface{})
			}
		}
	}

	inventory := map[string]interface{}{
		"all": map[string]interface{}{
			"hosts": allHosts,
			"vars": map[string]interface{}{
				ansibleSSHCommonArgsKey: ansibleSSHCommonArgs,
			},
		},
	}

	for group, hosts := range groupsMap {
		inventory[group] = map[string]interface{}{
			"hosts": hosts,
		}
	}

	return inventory
}

func (a *AnsibleInventory) buildInventoryStructure(vms []v1alpha2.VirtualMachine) map[string]interface{} {
	inventory := map[string]interface{}{
		"_meta": map[string]interface{}{
			"hostvars": make(map[string]interface{}),
		},
		"all": map[string]interface{}{
			"hosts": []string{},
			"vars": map[string]interface{}{
				ansibleSSHCommonArgsKey: ansibleSSHCommonArgs,
			},
		},
	}

	for _, vm := range vms {
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

	return inventory
}

func (a *AnsibleInventory) getVMGroups(vm v1alpha2.VirtualMachine) []string {
	if vm.Annotations == nil {
		return []string{}
	}

	groupsValue, exists := vm.Annotations[groupsAnnotationKey]
	if !exists || groupsValue == "" {
		return []string{}
	}

	groups := strings.Split(groupsValue, ",")
	result := make([]string, 0, len(groups))
	for _, group := range groups {
		group = strings.TrimSpace(group)
		if group != "" {
			result = append(result, group)
		}
	}

	return result
}

func (a *AnsibleInventory) generateHostInfo(vms []v1alpha2.VirtualMachine, hostName string) string {
	for _, vm := range vms {
		vmHostName := a.getHostName(vm)
		// Support search by both full name (vmname.namespace) and short name (vmname)
		if (vmHostName == hostName || vm.Name == hostName) && vm.Status.IPAddress != "" {
			hostVars := a.getHostVars(vm)

			var output []byte
			var err error
			switch strings.ToLower(a.options.Output) {
			case "yaml":
				output, err = yaml.Marshal(hostVars)
			default:
				output, err = json.MarshalIndent(hostVars, "", "  ")
			}
			if err != nil {
				return ""
			}

			return string(output)
		}
	}
	return ""
}

func (a *AnsibleInventory) getHostName(vm v1alpha2.VirtualMachine) string {
	return fmt.Sprintf("%s.%s", vm.Name, vm.Namespace)
}

func (a *AnsibleInventory) getHostVars(vm v1alpha2.VirtualMachine) map[string]interface{} {
	hostVars := map[string]interface{}{}

	// Add annotations as host variables
	// Only process annotations with prefix provisioning.virtualization.deckhouse.io/
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
  {{ProgramName}} ansible-inventory --host myvm.default

  # Specify namespace:
  {{ProgramName}} ansible-inventory --list -n mynamespace
  {{ProgramName}} ansible-inventory --host myvm -n mynamespace

  # Host names format: vmname.namespace (e.g., myvm.default)
  # VM annotations with prefix provisioning.virtualization.deckhouse.io/ are included as host variables
  # Use provisioning.virtualization.deckhouse.io/groups annotation to add VMs to groups
  #
  # Network access:
  # For VM access, the Default network interface is used. The plugin does not currently
  # support generating inventory for additional network interfaces.
  #
  # Examples of using annotations for Ansible configuration:
  #
  # Add VM to groups (comma-separated):
  #   kubectl annotate vm myvm provisioning.virtualization.deckhouse.io/groups="web,production" -n default
  #
  # Note: ansible_ssh_common_args is automatically set for port-forwarding through kubectl`
}
