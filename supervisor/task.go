package supervisor

import (
	"fmt"
	"reflect"
	"runtime"
	"strings"

	"git.tatikoma.dev/corpix/atlas/errors"
)

type (
	Task struct {
		ctx  Context
		fn   Job
		done chan void
	}
	Tasks []*Task

	Job func(ctx Context) error
	Loc struct {
		Package  string
		FuncName string
		File     string
		Line     int
	}
	Error struct {
		Err  error
		task *Task
	}
)

func (t *Task) Loc() (Loc, error) {
	v := reflect.ValueOf(t.fn)
	if v.Kind() != reflect.Func {
		return Loc{}, fmt.Errorf("expected a function, got %v", v.Kind())
	}
	pc := v.Pointer()
	if pc == 0 {
		return Loc{}, fmt.Errorf("invalid function pointer")
	}
	runtimeFunc := runtime.FuncForPC(pc)
	if runtimeFunc == nil {
		return Loc{}, fmt.Errorf("could not find function for PC")
	}

	var (
		file, line            = runtimeFunc.FileLine(pc)
		fullName              = runtimeFunc.Name()
		packageName, funcName string
	)
	if idx := strings.LastIndex(fullName, "."); idx != -1 {
		packageName, funcName = fullName[:idx], fullName[idx+1:]
	}

	return Loc{
		Package:  packageName,
		FuncName: funcName,
		File:     file,
		Line:     line,
	}, nil
}

func (l Loc) String() string {
	return fmt.Sprintf("%s.%s.%s:%d", l.File, l.Package, l.FuncName, l.Line)
}

func (e Error) Is(target error) bool {
	return errors.Is(e.Err, target)
}

func (e Error) Error() string {
	loc, err := e.task.Loc()
	var locStr string
	if err == nil {
		locStr = loc.String()
	} else {
		locStr = err.Error()
	}
	return fmt.Sprintf("task %s failed: %s", locStr, e.Err)
}
