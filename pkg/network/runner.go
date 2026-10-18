package network

import "os/exec"

// CommandRunner executes system commands. Tests inject a fake to avoid
// touching the host network configuration.
type CommandRunner interface {
	Run(name string, args ...string) ([]byte, error)
}

type execRunner struct{}

func (execRunner) Run(name string, args ...string) ([]byte, error) {
	return exec.Command(name, args...).CombinedOutput()
}
