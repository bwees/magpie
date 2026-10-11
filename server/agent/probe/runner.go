package probe

import (
	"bytes"
	"context"
	"os/exec"
)

// Runner runs an external program. Tests replace it with recorded ffmpeg and nvidia-smi output.
type Runner interface {
	// Run returns the program's stdout and stderr; err is non-nil when it could not start or exited non-zero.
	Run(ctx context.Context, name string, args ...string) (stdout, stderr string, err error)
}

type ExecRunner struct{}

func (ExecRunner) Run(ctx context.Context, name string, args ...string) (string, string, error) {
	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return stdout.String(), stderr.String(), err
}
