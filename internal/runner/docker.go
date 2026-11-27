package runner

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"time"

	"kebabseller6/rwe/internal/service"
)

const (
	workspaceMount  = "/workspace"
	pidsLimit       = "1024"
	dockerErrorCode = 125
	removeTimeout   = 30 * time.Second
)

type Docker struct {
	binary string
}

func NewDocker() *Docker {
	return &Docker{binary: "docker"}
}

func (d *Docker) Run(ctx context.Context, spec service.RunSpec, logs io.Writer) (int, error) {
	args, err := runArgs(spec)
	if err != nil {
		return -1, err
	}

	if spec.Timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, spec.Timeout)
		defer cancel()
	}

	cmd := exec.Command(d.binary, args...)
	cmd.Stdout = logs
	cmd.Stderr = logs
	if err := cmd.Start(); err != nil {
		return -1, fmt.Errorf("start docker: %w", err)
	}

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	select {
	case err := <-done:
		return exitCode(err)
	case <-ctx.Done():
		cmd.Process.Kill()
		<-done
		d.remove(spec.Name)
		return -1, fmt.Errorf("job %s stopped: %w", spec.Name, ctx.Err())
	}
}

func runArgs(spec service.RunSpec) ([]string, error) {
	if spec.Name == "" || spec.Image == "" || len(spec.Command) == 0 {
		return nil, errors.New("run spec needs a name, image and command")
	}
	filesDir, err := filepath.Abs(spec.FilesDir)
	if err != nil {
		return nil, err
	}

	args := []string{
		"run", "--rm",
		"--name", spec.Name,
		"--init",
		"--user", fmt.Sprintf("%d:%d", os.Getuid(), os.Getgid()),
		"--env", "HOME=/tmp",
		"--security-opt", "no-new-privileges",
		"--pids-limit", pidsLimit,
		"--volume", filesDir + ":" + workspaceMount,
		"--workdir", workspaceMount,
	}
	if spec.Memory > 0 {
		args = append(args, "--memory", strconv.FormatInt(spec.Memory, 10))
	}
	if spec.CPUs > 0 {
		args = append(args, "--cpus", strconv.FormatFloat(spec.CPUs, 'f', -1, 64))
	}
	args = append(args, spec.Image)
	return append(args, spec.Command...), nil
}

func exitCode(err error) (int, error) {
	if err == nil {
		return 0, nil
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		return -1, err
	}
	if code := exitErr.ExitCode(); code != dockerErrorCode {
		return code, nil
	}
	return -1, errors.New("docker failed to run the container (exit 125), see job log")
}

func (d *Docker) remove(name string) {
	ctx, cancel := context.WithTimeout(context.Background(), removeTimeout)
	defer cancel()
	exec.CommandContext(ctx, d.binary, "rm", "--force", name).Run()
}
