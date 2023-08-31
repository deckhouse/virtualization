package v2alpha1

const (
	// VMI has unexpected state
	ReasonErrUnknownState = "ErrUnknownState"

	// If importer/uploader Pod has error.
	ReasonErrImportFailed = "ErrImportFailed"

	// Error fetching progress metrics from Pod
	ReasonErrGetProgressFailed = "ErrGetProgressFailed"

	// Import completed successfully
	ReasonImportSucceeded = "ImportSucceeded"

	// When copy capacity from PVC successfully
	ReasonImportSucceededToPVC = "ImportSucceededToPVC"
)
