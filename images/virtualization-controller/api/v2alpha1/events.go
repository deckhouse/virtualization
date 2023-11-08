package v2alpha1

const (
	// ReasonErrUnknownState is event reason that VMI has unexpected state
	ReasonErrUnknownState = "ErrUnknownState"

	// ReasonErrWrongPVCSize is event reason that PVC has wrong size
	ReasonErrWrongPVCSize = "ErrWrongPVCSize"

	// ReasonErrImportFailed is event reason that importer/uploader Pod is failed
	ReasonErrImportFailed = "ErrImportFailed"

	// ReasonErrGetProgressFailed is event reason about the failure of getting progress.
	ReasonErrGetProgressFailed = "ErrGetProgressFailed"

	// ReasonImportSucceeded is event reason that the import is successfully completed
	ReasonImportSucceeded = "ImportSucceeded"

	// ReasonImportSucceededToPVC is event reason that the import is successfully completed to PVC
	ReasonImportSucceededToPVC = "ImportSucceededToPVC"

	// ReasonHotplugPostponed is event reason that disk hotplug is not possible at the moment.
	ReasonHotplugPostponed = "HotplugPostponed"

	// ReasonVMChangeIDExpired is event reason that change id approve request should be updated.
	ReasonVMChangeIDExpired = "ChangeIDExpired"

	// ReasonVMChangeIDApproveAccepted is event reason that change id approve was accepted and handled.
	ReasonVMChangeIDApproveAccepted = "ChangeIDApproveAccepted"

	// ReasonVMWaitForBlockDevices is event reason that block devices used by VM are not ready yet.
	ReasonVMWaitForBlockDevices = "WaitForBlockDevices"

	// ReasonVMChangesApplied is event reason that changes applied from VM to underlying KVVM.
	ReasonVMChangesApplied = "ChangesApplied"

	// ReasonVMRestarted is event reason that VM restarted.
	ReasonVMRestarted = "VMRestarted"
)
