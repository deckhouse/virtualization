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
)
