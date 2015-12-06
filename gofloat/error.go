package main

import (
	"fmt"
	"path/filepath"
	"runtime"
)

// errorContext wraps error with context information
// (file name, function name, line number)
type errorContext struct {
	Err  error
	File string
	Func string
	Line int
	Ok   bool
}

func (e errorContext) Error() string {
	if !e.Ok {
		return e.Err.Error()
	}
	return fmt.Sprintf("%s(%s:%d) %s", e.Func, filepath.Base(e.File),
		e.Line, e.Err)
}

func ctx(err error, skip int) error {
	pc, file, line, ok := runtime.Caller(skip)
	if !ok {
		return err
	}
	return errorContext{
		Err:  err,
		File: file,
		Func: runtime.FuncForPC(pc).Name(),
		Line: line,
		Ok:   ok}
}

func context(err error) error {
	if err == nil {
		return nil
	}
	return ctx(err, 2)
}

func contextErr(msg string, a ...interface{}) error {
	err := fmt.Errorf(msg, a...)
	return ctx(err, 2)
}
