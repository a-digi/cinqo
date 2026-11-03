//go:build !unix

package localai

import (
	"errors"
	"os/exec"
)

var errNoProcesses = errors.New("running the model is not supported on this system yet")

func setProcAttr(*exec.Cmd)         {}
func terminateGroup(int) error      { return errNoProcesses }
func killGroup(int) error           { return errNoProcesses }
func processAlive(int) bool         { return false }
func commandOf(int) (string, error) { return "", errNoProcesses }
