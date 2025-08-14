package common

import "errors"

var (
	ErrVirtualDiskSnapshotNotFound = errors.New("not found")
	ErrAlreadyExists               = errors.New("already exists")
	ErrAlreadyExistsAndHasDiff     = errors.New("already exists and does not have the same data content")
	ErrAlreadyInUse                = errors.New("already in use")
	ErrRestoring                   = errors.New("will be restored")
	ErrUpdating                    = errors.New("will be updated")
	ErrIncomplete                  = errors.New("still incomplete")
)
