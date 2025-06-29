package vdexportcondition

type Type string

func (t Type) String() string {
	return string(t)
}

const (
	TypeCompleted Type = "Completed"
)

type Reason string

func (r Reason) String() string {
	return string(r)
}

const (
	ReasonPending             Reason = "Pending"
	ReasonWaitForUserDownload Reason = "WaitForUserDownload"
	ReasonInProgress          Reason = "InProgress"
	ReasonCompleted           Reason = "Completed"
	ReasonExpired             Reason = "Expired"
	ReasonFailed              Reason = "Failed"
)
