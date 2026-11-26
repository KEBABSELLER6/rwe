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
  rwe run [-i IMAGE] [-t SECONDS] -- CMD...   sync the current dir and run CMD remotely
  rwe version`

type config struct {
	server string
	image  string
}

func loadConfig() config {
	return config{
		server: envOr("RWE_URL", "http://localhost:8999"),
		image:  envOr("RWE_IMAGE", "golang:1.27"),
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, usage)
		os.Exit(2)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	cfg := loadConfig()
	var err error
	code := 0
	switch os.Args[1] {
	case "mcp":
		err = serveMCP(ctx, cfg)
	case "run":
		code, err = runCmd(ctx, cfg, os.Args[2:])
	case "version":
		fmt.Println(version)
	default:
		fmt.Fprintln(os.Stderr, usage)
		code = 2
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "rwe:", err)
		code = 125
	}
	os.Exit(code)
}

func runCmd(ctx context.Context, cfg config, args []string) (int, error) {
	fs := flag.NewFlagSet("run", flag.ExitOnError)
	image := fs.String("i", cfg.image, "container image")
	timeout := fs.Int("t", 0, "timeout in seconds (0 = server default)")
	fs.Parse(args)
	if fs.NArg() == 0 {
		fmt.Fprintln(os.Stderr, usage)
		return 2, nil
	}

	dir, err := os.Getwd()
	if err != nil {
		return 0, err
	}

	c := client.New(cfg.server)
	job, err := c.Run(ctx, dir, client.JobSpec{Image: *image, Command: fs.Args(), TimeoutSeconds: *timeout}, func(line string) {
		fmt.Println(line)
	})
	if err != nil {
		return 0, err
	}

	fmt.Fprintf(os.Stderr, "==> %s\n", summary(job))
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
