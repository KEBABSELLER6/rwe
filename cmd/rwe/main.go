package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"kebabseller6/rwe/internal/client"
)

var version = "dev"

const usage = `usage:
  rwe mcp                                     serve MCP over stdio
  rwe run [-i IMAGE] [-t SECONDS] [-a PATH]... -- CMD...
                                              sync the current dir and run CMD remotely;
                                              -a brings PATH back to .rwe/artifacts/<job>/
  rwe version`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, usage)
		os.Exit(2)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	code, err := dispatch(ctx, os.Args[1], os.Args[2:])
	if err != nil {
		fmt.Fprintln(os.Stderr, "rwe:", err)
		code = 125
	}
	os.Exit(code)
}

func dispatch(ctx context.Context, cmd string, args []string) (int, error) {
	switch cmd {
	case "version":
		fmt.Println(version)
		return 0, nil
	case "mcp", "run":
	default:
		fmt.Fprintln(os.Stderr, usage)
		return 2, nil
	}

	dir, err := os.Getwd()
	if err != nil {
		return 0, err
	}
	cfg, err := loadConfig(dir)
	if err != nil {
		return 0, err
	}

	if cmd == "mcp" {
		return 0, serveMCP(ctx, cfg)
	}
	return runCmd(ctx, cfg, args)
}

func runCmd(ctx context.Context, cfg config, args []string) (int, error) {
	fs := flag.NewFlagSet("run", flag.ExitOnError)
	image := fs.String("i", cfg.Image, "container image")
	timeout := fs.Int("t", 0, "timeout in seconds (0 = server default)")
	var artifacts []string
	fs.Func("a", "workspace path to bring back, repeatable; replaces artifacts in .rwe.json", func(p string) error {
		artifacts = append(artifacts, p)
		return nil
	})
	fs.Parse(args)
	if artifacts == nil {
		artifacts = cfg.Artifacts
	}
	if fs.NArg() == 0 {
		fmt.Fprintln(os.Stderr, usage)
		return 2, nil
	}

	c := client.New(cfg.Server)
	spec := client.JobSpec{Image: *image, Command: fs.Args(), TimeoutSeconds: *timeout, Caches: cfg.Caches, Artifacts: artifacts}
	job, err := c.Run(ctx, cfg.dir, spec, func(line string) {
		fmt.Println(line)
	})
	if err != nil {
		return 0, err
	}

	fmt.Fprintf(os.Stderr, "==> %s\n", summary(job))
	if dst, err := c.FetchArtifacts(ctx, cfg.dir, job); err != nil {
		fmt.Fprintf(os.Stderr, "==> artifacts: %v\n", err)
	} else if dst != "" {
		fmt.Fprintf(os.Stderr, "==> artifacts in %s\n", dst)
	}
	return exitCode(job), nil
}

func exitCode(job client.Job) int {
	switch job.Status {
	case client.JobPassed:
		return 0
	case client.JobFailed:
		if job.ExitCode == nil {
			return 125
		}
		return *job.ExitCode
	case client.JobTimeout:
		return 124
	default:
		return 125
	}
}

func summary(job client.Job) string {
	s := fmt.Sprintf("%s in %.1fs", job.Status, float64(job.DurationMS)/1000)
	if job.Status == client.JobFailed && job.ExitCode != nil {
		s = fmt.Sprintf("%s (exit %d) in %.1fs", job.Status, *job.ExitCode, float64(job.DurationMS)/1000)
	}
	if job.Error != "" {
		s += ": " + job.Error
	}
	return s
}
