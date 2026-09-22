package supervisor

import (
	"fmt"

	"git.tatikoma.dev/corpix/atlas/errors"
)

type (
	task struct {
		exec Exec
		name string
	}

	Error struct {
		Err   error
		group string
		name  string
	}
)

func (e Error) Is(target error) bool {
	return errors.Is(e.Err, target)
}

func (e Error) Unwrap() error {
	return e.Err
}

func (e Error) Error() string {
	if e.group == "" {
		return fmt.Sprintf("task %s failed: %s", e.name, e.Err)
	}
	return fmt.Sprintf("task %s/%s failed: %s", e.group, e.name, e.Err)
}
