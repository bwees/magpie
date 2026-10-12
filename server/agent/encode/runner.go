package encode

import (
	"bufio"
	"bytes"
	"context"
	"io"
	"os/exec"
	"strings"
	"sync"
)

// Command is one program run. OnLine, when set, receives each line the program writes to stdout as it
// arrives, for ffmpeg's -progress output.
type Command struct {
	Name   string
	Args   []string
	OnLine func(line string)
}

func (c Command) String() string {
	return c.Name + " " + strings.Join(c.Args, " ")
}

// Runner runs ffmpeg and ffprobe. Tests replace it with a fake that answers from recorded output.
type Runner interface {
	// Run returns the program's stdout and stderr; err is set when it could not start or exited non-zero.
	Run(ctx context.Context, cmd Command) (stdout, stderr string, err error)
}

type ExecRunner struct{}

func (ExecRunner) Run(ctx context.Context, c Command) (string, string, error) {
	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, c.Name, c.Args...)
	cmd.Stderr = &stderr
	if c.OnLine == nil {
		cmd.Stdout = &stdout
		err := cmd.Run()
		return stdout.String(), stderr.String(), err
	}
	pipe, err := cmd.StdoutPipe()
	if err != nil {
		return "", "", err
	}
	if err := cmd.Start(); err != nil {
		return "", "", err
	}
	var wg sync.WaitGroup
	wg.Go(func() {
		scanner := bufio.NewScanner(io.TeeReader(pipe, &stdout))
		for scanner.Scan() {
			c.OnLine(scanner.Text())
		}
	})
	wg.Wait()
	err = cmd.Wait()
	return stdout.String(), stderr.String(), err
}
