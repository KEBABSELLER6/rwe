package runner

import (
	"context"
	"encoding/json/v2"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"kebabseller6/rwe/internal/service"
)

const (
	pidsLimit     = "1024"
	removeTimeout = 30 * time.Second
	ownerLabel    = "rwe.owner"
)

type Docker struct {
	binary string
	owner  string
}

func NewDocker(owner string) *Docker {
	return &Docker{binary: "docker", owner: owner}
}

func (d *Docker) RemoveStale(ctx context.Context) (int, error) {
	out, err := exec.CommandContext(ctx, d.binary, "ps", "--all", "--quiet", "--filter", "label="+ownerLabel+"="+d.owner).Output()
	if err != nil {
		return 0, fmt.Errorf("list stale containers: %w", err)
	}
	ids := strings.Fields(string(out))
	if len(ids) == 0 {
		return 0, nil
	}
	if out, err := exec.CommandContext(ctx, d.binary, append([]string{"rm", "--force"}, ids...)...).CombinedOutput(); err != nil {
		return 0, fmt.Errorf("remove stale containers: %w: %s", err, out)
	}
	return len(ids), nil
}

func (d *Docker) Run(ctx context.Context, spec service.RunSpec, logs io.Writer) (int, error) {
	args, err := runArgs(spec, d.owner)
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
	defer d.remove(spec.Name)

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	select {
	case err := <-done:
		return d.exitCode(spec.Name, err)
	case <-ctx.Done():
		cmd.Process.Kill()
		<-done
		return -1, fmt.Errorf("job %s stopped: %w", spec.Name, ctx.Err())
	}
}

func runArgs(spec service.RunSpec, owner string) ([]string, error) {
	if spec.Name == "" || spec.Image == "" || len(spec.Command) == 0 {
		return nil, errors.New("run spec needs a name, image and command")
	}
	filesDir, err := filepath.Abs(spec.FilesDir)
	if err != nil {
		return nil, err
	}

	args := []string{
		"run",
		"--name", spec.Name,
		"--label", ownerLabel + "=" + owner,
		"--init",
		"--user", fmt.Sprintf("%d:%d", os.Getuid(), os.Getgid()),
		"--env", "HOME=/tmp",
		"--security-opt", "no-new-privileges",
		"--pids-limit", pidsLimit,
		"--volume", filesDir + ":" + service.WorkspaceMount,
		"--workdir", service.WorkspaceMount,
	}
	for _, c := range spec.Caches {
		hostDir, err := filepath.Abs(c.HostDir)
		if err != nil {
			return nil, err
		}
		args = append(args, "--volume", hostDir+":"+c.Path)
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

func (d *Docker) exitCode(name string, err error) (int, error) {
	if err == nil {
		return 0, nil
	}
	var exitErr *exec.ExitError
	if !errors.As(err, &exitErr) {
		return -1, err
	}
	code := exitErr.ExitCode()
	if !dockerOwnCode(code) || d.ranToExit(name, code) {
		return code, nil
	}
	return -1, fmt.Errorf("docker failed to run the container (exit %d), see job log", code)
}

func dockerOwnCode(code int) bool {
	return code == 125 || code == 126 || code == 127
}

func (d *Docker) ranToExit(name string, code int) bool {
	out, err := exec.Command(d.binary, "inspect", "--format", "{{json .State}}", name).Output()
	if err != nil {
		return false
	}
	var state struct {
		StartedAt string
		ExitCode  int
		Error     string
	}
	if json.Unmarshal(out, &state) != nil {
		return false
	}
	started := state.StartedAt != "" && !strings.HasPrefix(state.StartedAt, "0001-01-01")
	return started && state.Error == "" && state.ExitCode == code
}

func (d *Docker) remove(name string) {
	ctx, cancel := context.WithTimeout(context.Background(), removeTimeout)
	defer cancel()
	exec.CommandContext(ctx, d.binary, "rm", "--force", name).Run()
}
