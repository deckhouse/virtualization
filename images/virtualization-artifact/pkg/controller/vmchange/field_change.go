package vmchange

type ChangeOperation string

const (
	ChangeNone    ChangeOperation = "none"
	ChangeAdd     ChangeOperation = "add"
	ChangeRemove  ChangeOperation = "remove"
	ChangeReplace ChangeOperation = "replace"
)

type ActionType string

const (
	ActionNone              ActionType = ""
	ActionRestart           ActionType = "Restart"
	ActionSubresourceSignal ActionType = "SubresourceSignal"
	ActionApplyImmediate    ActionType = "ApplyImmediate"
)

type FieldChange struct {
	Operation    ChangeOperation `json:"operation,omitempty"`
	Path         string          `json:"path,omitempty"`
	CurrentValue interface{}     `json:"currentValue,omitempty"`
	DesiredValue interface{}     `json:"desiredValue,omitempty"`

	ActionRequired ActionType `json:"-"`
}

func HasChanges(changes []FieldChange) bool {
	for _, change := range changes {
		if change.Operation != ChangeNone {
			return true
		}
	}
	return false
}
