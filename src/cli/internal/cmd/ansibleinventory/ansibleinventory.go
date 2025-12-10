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
	"bytes"
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

type inventoryData struct {
	hostVars map[string]map[string]string // hostname -> variables
	groups   map[string][]string          // group -> list of hostnames
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
		Output:    "json",
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

When called without arguments, returns all hosts (same as --list).

Only virtual machines with assigned IP addresses are included in the inventory.

Arguments:
  --list                   Return all hosts (default behavior if no arguments)
  --host <hostname>        Show variables for a particular host; output format matches inventory entries
  --output, -o <format>    Output format: json, ini, or yaml (default: json)
  --namespace, -n <ns>     Namespace to list virtual machines from
                          (overrides kubeconfig context namespace)

Host names format: <vmname>.<namespace> (e.g., myvm.default)

VM annotations:
  - Annotations with prefix 'provisioning.virtualization.deckhouse.io/' are included
    as host variables (prefix is stripped from variable name)
  - Use 'provisioning.virtualization.deckhouse.io/groups' annotation to add VMs to groups
    (comma-separated list of group names)

Network access:
  - For VM access, the Default network interface is used
  - The 'ansible_ssh_common_args' variable is automatically set for port-forwarding
    through kubectl using 'd8 v port-forward' command
  - Additional network interfaces are not currently supported`,
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
		"Output format: json, ini, or yaml (default: json)")
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

	if a.options.Host != "" {
		nsFromHost, hostName := a.parseHost(a.options.Host)

		if nsFromHost == "" && namespace == "" {
			return fmt.Errorf("no default namespace in context, no --namespace arg, no namespace in --host: specify namespace for host info")
		}

		// Override namespace if the `--host` argument is in the form host.namespace.
		if nsFromHost != "" {
			namespace = nsFromHost
		}

		vm, err := client.VirtualMachines(namespace).Get(context.TODO(), hostName, metav1.GetOptions{})
		if err != nil {
			return fmt.Errorf("get vm %s in namespace %s: %w", hostName, namespace, err)
		}

		hostInfo := a.generateHostInfo(vm)
		if hostInfo == "" {
			cmd.Print("{}")
			return nil
		}
		cmd.Print(hostInfo)
		return nil
	}

	if a.options.List {
		if namespace == "" {
			return fmt.Errorf("no default namespace in context, no --namespace arg: inventory for all VirtualMachines is not implemented yet, specify namespace")
		}
		vmList, err := client.VirtualMachines(namespace).List(context.TODO(), metav1.ListOptions{})
		if err != nil {
			return fmt.Errorf("failed to list virtual machines: %w", err)
		}
		inventory := a.generateInventory(vmList.Items)
		cmd.Print(inventory)
		return nil
	}

	return nil
}

// ============================================================================
// Inventory generation
// ============================================================================

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

func (a *AnsibleInventory) generateInventoryINI(vms []v1alpha2.VirtualMachine) string {
	data := a.collectInventoryData(vms)

	var builder strings.Builder

	builder.WriteString("[all]\n")
	for hostName, hostVars := range data.hostVars {
		builder.WriteString(hostName)
		for varName, value := range hostVars {
			builder.WriteString(fmt.Sprintf(" %s=%s", varName, value))
		}
		builder.WriteString("\n")
	}

	builder.WriteString("\n[all:vars]\n")
	builder.WriteString(fmt.Sprintf("%s=\"%s\"\n", ansibleSSHCommonArgsKey, ansibleSSHCommonArgs))

	for group, hosts := range data.groups {
		builder.WriteString(fmt.Sprintf("\n[%s]\n", group))
		for _, host := range hosts {
			builder.WriteString(fmt.Sprintf("%s\n", host))
		}
	}

	return builder.String()
}

func (a *AnsibleInventory) generateInventoryYAML(vms []v1alpha2.VirtualMachine) string {
	data := a.collectInventoryData(vms)
	inventory := a.buildYAMLInventory(data)

	output, err := yaml.Marshal(inventory)
	if err != nil {
		return fmt.Sprintf("Error: %v", err)
	}

	return string(output)
}

func (a *AnsibleInventory) generateInventoryJSON(vms []v1alpha2.VirtualMachine) string {
	data := a.collectInventoryData(vms)
	inventory := a.buildJSONInventory(data)

	output, err := json.MarshalIndent(inventory, "", "  ")
	if err != nil {
		return fmt.Sprintf("Error: %v", err)
	}

	return string(output)
}

// ============================================================================
// Data collection and structure building
// ============================================================================

func (a *AnsibleInventory) collectInventoryData(vms []v1alpha2.VirtualMachine) inventoryData {
	data := inventoryData{
		hostVars: make(map[string]map[string]string),
		groups:   make(map[string][]string),
	}

	for _, vm := range vms {
		if !a.isValidVM(vm) {
			continue
		}

		hostName := a.getHostName(vm)
		hostVars := a.getHostVars(vm)
		if hostVars == nil {
			hostVars = make(map[string]string)
		}
		data.hostVars[hostName] = hostVars

		groups := a.getVMGroups(vm)
		for _, group := range groups {
			data.groups[group] = append(data.groups[group], hostName)
		}
	}

	return data
}

func (a *AnsibleInventory) buildYAMLInventory(data inventoryData) map[string]interface{} {
	allHosts := make(map[string]interface{})
	groupsMap := make(map[string]map[string]interface{})

	for hostName, hostVars := range data.hostVars {
		allHosts[hostName] = hostVars
	}

	for group, hostNames := range data.groups {
		groupsMap[group] = make(map[string]interface{})
		for _, hostName := range hostNames {
			groupsMap[group][hostName] = data.hostVars[hostName]
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

func (a *AnsibleInventory) buildJSONInventory(data inventoryData) map[string]interface{} {
	allHosts := make([]string, 0, len(data.hostVars))
	for hostName := range data.hostVars {
		allHosts = append(allHosts, hostName)
	}

	inventory := map[string]interface{}{
		"_meta": map[string]interface{}{
			"hostvars": data.hostVars,
		},
		"all": map[string]interface{}{
			"hosts": allHosts,
			"vars": map[string]interface{}{
				ansibleSSHCommonArgsKey: ansibleSSHCommonArgs,
			},
		},
	}

	for group, hostNames := range data.groups {
		inventory[group] = map[string]interface{}{
			"hosts": hostNames,
		}
	}

	return inventory
}

// ============================================================================
// VM helper methods
// ============================================================================

func (a *AnsibleInventory) isValidVM(vm v1alpha2.VirtualMachine) bool {
	return vm.Status.IPAddress != "" && vm.Status.Phase == v1alpha2.MachineRunning
}

func (a *AnsibleInventory) getHostName(vm v1alpha2.VirtualMachine) string {
	return fmt.Sprintf("%s.%s", vm.Name, vm.Namespace)
}

func (a *AnsibleInventory) getHostVars(vm v1alpha2.VirtualMachine) map[string]string {
	hostVars := make(map[string]string)

	// Add annotations as host variables
	// Only process annotations with prefix provisioning.virtualization.deckhouse.io/
	if len(vm.Annotations) > 0 {
		for key, value := range vm.Annotations {
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

// ============================================================================
// Host info generation
// ============================================================================

// parseHost returns namespace and name for the --host option:
//
// - "hostname" form: namespace is empty string, name is hostname
// - "hostname.namespace" form: split this by . and return namespace and name.
func (a *AnsibleInventory) parseHost(hostOpt string) (string, string) {
	name, namespace, _ := strings.Cut(hostOpt, ".")
	return namespace, name
}

func (a *AnsibleInventory) generateHostInfo(vm *v1alpha2.VirtualMachine) string {
	hostVars := a.getHostVars(*vm)

	var output []byte
	var err error
	switch strings.ToLower(a.options.Output) {
	case "yaml":
		output, err = yaml.Marshal(hostVars)
	case "ini":
		var builder bytes.Buffer
		first := true
		for varName, value := range hostVars {
			if first {
				first = false
			} else {
				builder.WriteString(" ")
			}
			builder.WriteString(fmt.Sprintf("%s=%s", varName, value))
		}
		builder.WriteString("\n")
		output = builder.Bytes()
	default:
		output, err = json.MarshalIndent(hostVars, "", "  ")
	}
	if err != nil {
		return ""
	}

	return string(output)
}

// ============================================================================
// Usage
// ============================================================================

func usage() string {
	return `  # Standard ansible inventory script interface:
  # Return all hosts (default format is JSON):
  {{ProgramName}} ansible-inventory [--list]
  {{ProgramName}} ansible-inventory [--list] -o json
  {{ProgramName}} ansible-inventory [--list] -o yaml
  {{ProgramName}} ansible-inventory [--list] -o ini

  # Return variables for specific host:
  # Supports both full name (vmname.namespace) and short name (vmname)
  {{ProgramName}} ansible-inventory --host myvm.default

  # Specify namespace:
  {{ProgramName}} ansible-inventory [--list] -n mynamespace
  {{ProgramName}} ansible-inventory --host myvm -n mynamespace

  # Examples of using annotations for Ansible configuration:
  #
  # Add VM to groups (comma-separated):
  #   kubectl annotate vm myvm provisioning.virtualization.deckhouse.io/groups="web,production" -n default
  #
  # Add custom host variable:
  #   kubectl annotate vm myvm provisioning.virtualization.deckhouse.io/ansible_user="admin" -n default
  #   # This will be available as 'ansible_user' variable in Ansible
  #
  # Note: Only VMs with assigned IP addresses are included in the inventory.
  #       The 'ansible_ssh_common_args' variable is automatically set for port-forwarding.`
}
