package kvbuilder

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strconv"
	"strings"

	corev1 "k8s.io/api/core/v1"
	virtv1 "kubevirt.io/api/core/v1"
)

type ActionType string

const (
	ActionNone              ActionType = ""
	ActionRestart           ActionType = "Restart"
	ActionSubresourceSignal ActionType = "SubresourceSignal"
	ActionApplyImmediate    ActionType = "ApplyImmediate"
)

// SubresourceSignal holds a payload to send to subresource to apply changes.
type SubresourceSignal struct {
	Subresource string
	Method      string
	Payload     map[string]interface{}
}

type ChangeItem struct {
	Title string `json:"title"`
	Curr  string `json:"curr"`
	Next  string `json:"next"`
}

func (ci ChangeItem) String() string {
	return fmt.Sprintf("%s: %s -> %s", ci.Title, ci.Curr, ci.Next)
}

type ChangeApplyAction struct {
	Type              ActionType
	Changes           []ChangeItem
	SubresourceSignal *SubresourceSignal
	Disruptive        bool
}

func (c *ChangeApplyAction) ChangeID() string {
	if len(c.Changes) == 0 {
		return "no-changes"
	}
	sort.SliceStable(c.Changes, func(i, j int) bool {
		return c.Changes[i].Title < c.Changes[j].Title
	})

	hasher := sha256.New()
	for _, ch := range c.Changes {
		hasher.Write([]byte(ch.String()))
	}

	return fmt.Sprintf("%x", hasher.Sum(nil))
}

func (c *ChangeApplyAction) ChangesString() string {
	if len(c.Changes) == 0 {
		return ""
	}
	sort.SliceStable(c.Changes, func(i, j int) bool {
		return c.Changes[i].Title < c.Changes[j].Title
	})

	buf := strings.Builder{}
	for _, ch := range c.Changes {
		buf.WriteString(ch.String())
		buf.WriteString("\n")
	}

	return buf.String()
}

type ChangeApplyActions struct {
	Actions []*ChangeApplyAction
}

func NewChangeApplyActions() *ChangeApplyActions {
	return &ChangeApplyActions{}
}

func (caa *ChangeApplyActions) IsEmpty() bool {
	return caa == nil || len(caa.Actions) == 0
}

func (caa *ChangeApplyActions) Add(action *ChangeApplyAction) {
	if action == nil {
		return
	}
	caa.Actions = append(caa.Actions, action)
}

// ActionType returns the most dangerous action type:
// None < ApplyImmediate < SubresourceSignal < Restart
func (caa *ChangeApplyActions) ActionType() ActionType {
	if caa == nil {
		return ActionNone
	}

	// Types from most dangerous to least dangerous.
	typesInOrder := []ActionType{
		ActionRestart,
		ActionSubresourceSignal,
		ActionApplyImmediate,
	}

	for _, typ := range typesInOrder {
		for _, action := range caa.Actions {
			if action.Type == typ {
				return typ
			}
		}
	}

	return ActionNone
}

func (caa *ChangeApplyActions) GetChangesTitles() []string {
	res := make([]string, 0)

	for _, action := range caa.Actions {
		for _, change := range action.Changes {
			res = append(res, change.Title)
		}
	}

	return res
}

func (caa *ChangeApplyActions) ChangeID() string {
	// Assume actions are always in same order: cpu, run policy, disks and so on.
	hasher := sha256.New()
	for _, action := range caa.Actions {
		hasher.Write([]byte(action.ChangeID()))
	}
	return fmt.Sprintf("%x", hasher.Sum(nil))
}

// IsDisruptive detects whether actions are disruptive.
func (caa *ChangeApplyActions) IsDisruptive() bool {
	if caa.ActionType() == ActionRestart {
		return true
	}
	for _, action := range caa.Actions {
		if action.Disruptive {
			return true
		}
	}
	return false
}

func CompareKVVM(curr, next *KVVM) (*ChangeApplyActions, error) {
	// Make kvvm from prev version.
	// Make kvvm from current version.
	// Compare kvvm's and determine the action to apply changes.
	// Also, calculate change ID.

	actions := NewChangeApplyActions()

	{
		action, err := CompareCPUModel(curr, next)
		if err != nil {
			return nil, err
		}
		actions.Add(action)
	}

	{
		action, err := CompareRunPolicy(curr, next)
		if err != nil {
			return nil, err
		}
		actions.Add(action)
	}

	{
		action, err := CompareResourceRequirements(curr, next)
		if err != nil {
			return nil, err
		}
		actions.Add(action)
	}

	{
		action, err := CompareDisks(curr, next)
		if err != nil {
			return nil, err
		}
		actions.Add(action)
	}

	{
		action, err := CompareTablet(curr, next)
		if err != nil {
			return nil, err
		}
		actions.Add(action)
	}

	{
		action, err := CompareCloudInit(curr, next)
		if err != nil {
			return nil, err
		}
		actions.Add(action)
	}

	{
		action, err := CompareOSType(curr, next)
		if err != nil {
			return nil, err
		}
		actions.Add(action)
	}

	{
		action, err := CompareNetworkInterfaces(curr, next)
		if err != nil {
			return nil, err
		}
		actions.Add(action)
	}

	{
		action, err := CompareBootloader(curr, next)
		if err != nil {
			return nil, err
		}
		actions.Add(action)
	}
	{
		action, err := CompareNodeSelector(curr, next)
		if err != nil {
			return nil, err
		}
		actions.Add(action)
	}
	{
		action, err := CompareTolerations(curr, next)
		if err != nil {
			return nil, err
		}
		actions.Add(action)
	}
	{
		action, err := CompareAffinity(curr, next)
		if err != nil {
			return nil, err
		}
		actions.Add(action)
	}
	{
		action, err := ComparePriorityClassName(curr, next)
		if err != nil {
			return nil, err
		}
		actions.Add(action)
	}
	{
		action, err := CompareTerminationGracePeriod(curr, next)
		if err != nil {
			return nil, err
		}
		actions.Add(action)
	}
	{
		action, err := CompareTopologySpreadConstraint(curr, next)
		if err != nil {
			return nil, err
		}
		actions.Add(action)
	}

	return actions, nil
}

// CompareCPUModel returns Restart action if CPU model is changed.
func CompareCPUModel(curr, next *KVVM) (*ChangeApplyAction, error) {
	if curr.Resource.Spec.Template.Spec.Domain.CPU.Model == next.Resource.Spec.Template.Spec.Domain.CPU.Model {
		return nil, nil
	}

	return &ChangeApplyAction{
		Type: ActionRestart,
		Changes: []ChangeItem{
			{
				Title: "CPU Model",
				Curr:  curr.Resource.Spec.Template.Spec.Domain.CPU.Model,
				Next:  next.Resource.Spec.Template.Spec.Domain.CPU.Model,
			},
		},
	}, nil
}

// CompareRunPolicy returns ApplyImmediate action if RunPolicy was changed to non-nil value.
func CompareRunPolicy(curr, next *KVVM) (*ChangeApplyAction, error) {
	// Next run policy is nil, no action required.
	if next.Resource.Spec.RunStrategy == nil {
		return nil, nil
	}
	nextRunPolicy := string(*next.Resource.Spec.RunStrategy)

	currRunPolicy := ""
	if curr.Resource.Spec.RunStrategy != nil {
		currRunPolicy = string(*curr.Resource.Spec.RunStrategy)
	}

	if currRunPolicy == nextRunPolicy {
		return nil, nil
	}

	return &ChangeApplyAction{
		Type: ActionApplyImmediate,
		Changes: []ChangeItem{
			{
				Title: "RunPolicy",
				Curr:  currRunPolicy,
				Next:  nextRunPolicy,
			},
		},
	}, nil
}

// CompareResourceRequirements returns Restart action of CPU or Memory limits are changed.
func CompareResourceRequirements(curr, next *KVVM) (*ChangeApplyAction, error) {
	changes := make([]ChangeItem, 0)

	currResources := curr.Resource.Spec.Template.Spec.Domain.Resources
	nextResources := next.Resource.Spec.Template.Spec.Domain.Resources

	if currResources.Requests[corev1.ResourceCPU] != nextResources.Requests[corev1.ResourceCPU] ||
		currResources.Limits[corev1.ResourceCPU] != nextResources.Limits[corev1.ResourceCPU] {
		changes = append(changes, ChangeItem{
			Title: "ResourceRequirements CPU",
			Curr:  currResources.Requests.Cpu().String(),
			Next:  nextResources.Requests.Cpu().String(),
		})
	}

	if currResources.Requests[corev1.ResourceMemory] != nextResources.Requests[corev1.ResourceMemory] ||
		currResources.Limits[corev1.ResourceMemory] != nextResources.Limits[corev1.ResourceMemory] {
		changes = append(changes, ChangeItem{
			Title: "ResourceRequirements Memory",
			Curr:  currResources.Requests.Memory().String(),
			Next:  nextResources.Requests.Memory().String(),
		})
	}

	// Ignore if no changes made to ResourceRequirements.
	if len(changes) == 0 {
		return nil, nil
	}

	return &ChangeApplyAction{
		Type:    ActionRestart,
		Changes: changes,
	}, nil
}

// CompareDisks returns Restart action if VM volumes are changed.
// TODO add meaningful diff messages.
// TODO add detailed changes to generate proper change ID.
// TODO detect disk or volume removing and set Disruptive to true.
func CompareDisks(curr, next *KVVM) (*ChangeApplyAction, error) {
	changes := make([]ChangeItem, 0)

	hotpluggedDisks := map[string]struct{}{}

	// Check if there are changes in VM volumes.
	{
		var currVolumes []virtv1.Volume
		for _, volume := range curr.Resource.Spec.Template.Spec.Volumes {
			if volume.PersistentVolumeClaim != nil && volume.PersistentVolumeClaim.Hotpluggable ||
				volume.DataVolume != nil && volume.DataVolume.Hotpluggable {
				hotpluggedDisks[volume.Name] = struct{}{}
				continue
			}
			currVolumes = append(currVolumes, volume)
		}

		var nextVolumes []virtv1.Volume
		for _, volume := range next.Resource.Spec.Template.Spec.Volumes {
			if volume.PersistentVolumeClaim != nil && volume.PersistentVolumeClaim.Hotpluggable ||
				volume.DataVolume != nil && volume.DataVolume.Hotpluggable {
				hotpluggedDisks[volume.Name] = struct{}{}
				continue
			}
			nextVolumes = append(nextVolumes, volume)
		}

		sort.SliceStable(currVolumes, func(i, j int) bool {
			return currVolumes[i].Name < currVolumes[j].Name
		})

		sort.SliceStable(nextVolumes, func(i, j int) bool {
			return nextVolumes[i].Name < nextVolumes[j].Name
		})

		if !reflect.DeepEqual(currVolumes, nextVolumes) {
			changes = append(changes, ChangeItem{
				Title: "Volumes",
				Curr:  "old",
				Next:  "new",
			})
		}
	}

	// Check if there are changes in VM disks.
	{
		var currDisks []virtv1.Disk
		for _, disk := range curr.Resource.Spec.Template.Spec.Domain.Devices.Disks {
			_, isHotplugged := hotpluggedDisks[disk.Name]
			if !isHotplugged {
				currDisks = append(currDisks, disk)
			}
		}

		var nextDisks []virtv1.Disk
		for _, disk := range next.Resource.Spec.Template.Spec.Domain.Devices.Disks {
			_, isHotplugged := hotpluggedDisks[disk.Name]
			if !isHotplugged {
				nextDisks = append(nextDisks, disk)
			}
		}

		sort.SliceStable(currDisks, func(i, j int) bool {
			return currDisks[i].Name < currDisks[j].Name
		})

		sort.SliceStable(nextDisks, func(i, j int) bool {
			return nextDisks[i].Name < nextDisks[j].Name
		})

		if !reflect.DeepEqual(currDisks, nextDisks) {
			changes = append(changes, ChangeItem{
				Title: "Disks",
				Curr:  "old",
				Next:  "new",
			})
		}
	}

	// Ignore if no changes made to Disks or Volumes.
	if len(changes) == 0 {
		return nil, nil
	}

	return &ChangeApplyAction{
		Type:    ActionRestart,
		Changes: changes,
	}, nil
}

// CompareTablet returns Restart action if tablet appears or disappears.
func CompareTablet(curr, next *KVVM) (*ChangeApplyAction, error) {
	currHasTablet := curr.HasTablet("default-0")
	nextHasTablet := next.HasTablet("default-0")

	// Ignore if no changes.
	if currHasTablet == nextHasTablet {
		return nil, nil
	}

	// TODO tablet is a USB device, is there a subresource signal to remove/add USB devices?
	return &ChangeApplyAction{
		Type: ActionRestart,
		Changes: []ChangeItem{
			{
				Title: "Tablet input connected",
				Curr:  strconv.FormatBool(currHasTablet),
				Next:  strconv.FormatBool(nextHasTablet),
			},
		},
	}, nil
}

func CompareCloudInit(curr, next *KVVM) (*ChangeApplyAction, error) {
	currSettings := curr.GetCloudInitSettings()
	nextSettings := next.GetCloudInitSettings()
	if reflect.DeepEqual(currSettings, nextSettings) {
		return nil, nil
	}
	return &ChangeApplyAction{
		Type: ActionRestart,
		Changes: []ChangeItem{
			{
				Title: CloudInitDiskName,
				Curr:  "old",
				Next:  "new",
			},
		},
	}, nil
}

// CompareOSType compares some devices and some features to detect changes on OSType change.
// TODO add meaningful diff message
// TODO add detailed changes to generate proper change ID.
func CompareOSType(curr, next *KVVM) (*ChangeApplyAction, error) {
	currSettings := curr.GetOSSettings()
	nextSettings := next.GetOSSettings()

	if reflect.DeepEqual(currSettings, nextSettings) {
		return nil, nil
	}

	return &ChangeApplyAction{
		Type: ActionRestart,
		Changes: []ChangeItem{
			{
				Title: "OS Type",
				Curr:  "old",
				Next:  "new",
			},
		},
	}, nil
}

// CompareNetworkInterfaces
// TODO add meaningful diff message
// TODO add detailed changes to generate proper change ID.
func CompareNetworkInterfaces(curr, next *KVVM) (*ChangeApplyAction, error) {
	currIfaces := curr.Resource.Spec.Template.Spec.Domain.Devices.Interfaces
	nextIfaces := next.Resource.Spec.Template.Spec.Domain.Devices.Interfaces
	sort.SliceStable(currIfaces, func(i, j int) bool {
		return currIfaces[i].Name < currIfaces[j].Name
	})
	sort.SliceStable(nextIfaces, func(i, j int) bool {
		return nextIfaces[i].Name < nextIfaces[j].Name
	})
	if reflect.DeepEqual(currIfaces, nextIfaces) {
		return nil, nil
	}

	return &ChangeApplyAction{
		Type: ActionRestart,
		Changes: []ChangeItem{
			{
				Title: "Network interfaces",
				Curr:  "old",
				Next:  "new",
			},
		},
	}, nil
}

// CompareBootloader
// TODO add meaningful diff message
// TODO add detailed changes to generate proper change ID.
func CompareBootloader(curr, next *KVVM) (*ChangeApplyAction, error) {
	currSettings := curr.GetBootloaderSettings()
	nextSettings := next.GetBootloaderSettings()

	if reflect.DeepEqual(currSettings, nextSettings) {
		return nil, nil
	}

	return &ChangeApplyAction{
		Type: ActionRestart,
		Changes: []ChangeItem{
			{
				Title: "Bootloader settings",
				Curr:  "old",
				Next:  "new",
			},
		},
	}, nil
}

func CompareNodeSelector(curr, next *KVVM) (*ChangeApplyAction, error) {
	currNS := curr.Resource.Spec.Template.Spec.NodeSelector
	nextNS := next.Resource.Spec.Template.Spec.NodeSelector
	if reflect.DeepEqual(currNS, nextNS) {
		return nil, nil
	}
	oldNS, err := json.Marshal(currNS)
	if err != nil {
		return nil, fmt.Errorf("comparison failed. %w", err)
	}
	newNS, err := json.Marshal(nextNS)
	if err != nil {
		return nil, fmt.Errorf("comparison failed. %w", err)
	}
	return &ChangeApplyAction{
		Type: ActionRestart,
		Changes: []ChangeItem{
			{
				Title: "NodeSelector",
				Curr:  string(oldNS),
				Next:  string(newNS),
			},
		},
	}, nil
}

// TODO add meaningful diff message
// TODO add detailed changes to generate proper change ID.
func CompareTolerations(curr, next *KVVM) (*ChangeApplyAction, error) {
	currT := curr.Resource.Spec.Template.Spec.Tolerations
	nextT := next.Resource.Spec.Template.Spec.Tolerations
	if reflect.DeepEqual(currT, nextT) {
		return nil, nil
	}
	return &ChangeApplyAction{
		Type: ActionRestart,
		Changes: []ChangeItem{
			{
				Title: "Tolerations",
				Curr:  "old",
				Next:  "new",
			},
		},
	}, nil
}

// TODO add meaningful diff message
// TODO add detailed changes to generate proper change ID.
func CompareAffinity(curr, next *KVVM) (*ChangeApplyAction, error) {
	currAff := curr.Resource.Spec.Template.Spec.Affinity
	nextAff := next.Resource.Spec.Template.Spec.Affinity
	if reflect.DeepEqual(currAff, nextAff) {
		return nil, nil
	}
	return &ChangeApplyAction{
		Type: ActionRestart,
		Changes: []ChangeItem{
			{
				Title: "Affinity",
				Curr:  "old",
				Next:  "new",
			},
		},
	}, nil
}

func ComparePriorityClassName(curr, next *KVVM) (*ChangeApplyAction, error) {
	oldPCN := curr.Resource.Spec.Template.Spec.PriorityClassName
	newPCN := next.Resource.Spec.Template.Spec.PriorityClassName
	if oldPCN == newPCN {
		return nil, nil
	}
	return &ChangeApplyAction{
		Type: ActionRestart,
		Changes: []ChangeItem{
			{
				Title: "PriorityClassName",
				Curr:  oldPCN,
				Next:  newPCN,
			},
		},
	}, nil
}

func CompareTerminationGracePeriod(curr, next *KVVM) (*ChangeApplyAction, error) {
	var oldPeriod string
	var newPeriod string
	if c := curr.Resource.Spec.Template.Spec.TerminationGracePeriodSeconds; c != nil {
		oldPeriod = strconv.FormatInt(*c, 10)
	}
	if n := next.Resource.Spec.Template.Spec.TerminationGracePeriodSeconds; n != nil {
		newPeriod = strconv.FormatInt(*n, 10)
	}
	if oldPeriod == newPeriod {
		return nil, nil
	}
	return &ChangeApplyAction{
		Type: ActionRestart,
		Changes: []ChangeItem{
			{
				Title: "TerminationGracePeriod",
				Curr:  oldPeriod,
				Next:  newPeriod,
			},
		},
	}, nil
}

// TODO add meaningful diff message
// TODO add detailed changes to generate proper change ID.
func CompareTopologySpreadConstraint(curr, next *KVVM) (*ChangeApplyAction, error) {
	currT := curr.Resource.Spec.Template.Spec.TopologySpreadConstraints
	nextT := next.Resource.Spec.Template.Spec.TopologySpreadConstraints
	if reflect.DeepEqual(currT, nextT) {
		return nil, nil
	}
	return &ChangeApplyAction{
		Type: ActionRestart,
		Changes: []ChangeItem{
			{
				Title: "TopologySpreadConstraint",
				Curr:  "old",
				Next:  "new",
			},
		},
	}, nil
}
