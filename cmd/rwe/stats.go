package main

import (
	"flag"
	"fmt"
	"slices"
	"strings"
	"text/tabwriter"
	"time"

	"kebabseller6/rwe/internal/client"
)

const defaultStatsRuns = 20

func statsCmd(cfg config, args []string) (int, error) {
	fs := flag.NewFlagSet("stats", flag.ExitOnError)
	n := fs.Int("n", defaultStatsRuns, "number of recent runs")
	fs.Parse(args)

	runs, err := client.LoadRuns(cfg.dir, *n)
	if err != nil {
		return 0, err
	}
	fmt.Print(formatStats(runs))
	return 0, nil
}

func formatStats(runs []client.RunRecord) string {
	if len(runs) == 0 {
		return "no runs recorded in .rwe/runs.jsonl yet\n"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "last %d runs: %s; total p50 %s, p90 %s\n", len(runs), statusCounts(runs), percentile(runs, 0.5), percentile(runs, 0.9))
	fmt.Fprintf(&b, "time spent: %s\n\n", timeSpent(runs))

	w := tabwriter.NewWriter(&b, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "when\tstatus\ttotal\tsync\tqueue\trun\tfiles\tsent\thash\tjob\tcommand")
	for _, r := range slices.Backward(runs) {
		sent := fmt.Sprint(r.Uploaded)
		if r.Created {
			sent = "new"
		}
		run := "-"
		if r.PhasesMS != nil {
			run = ms(r.PhasesMS["run"])
		}
		fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\t%s\t%d\t%s\t%.12s\t%s\t%s\n",
			r.At.Local().Format("01-02 15:04:05"), runStatus(r), ms(r.TotalMS), ms(r.SyncMS), ms(r.QueueMS), run,
			r.Files, sent, r.Hash, r.JobID, strings.Join(r.Command, " "))
	}
	w.Flush()

	var errs []string
	for _, r := range slices.Backward(runs) {
		if r.Error != "" {
			errs = append(errs, fmt.Sprintf("  %s %s: %s\n", r.At.Local().Format("01-02 15:04:05"), r.JobID, r.Error))
		}
	}
	if errs != nil {
		fmt.Fprintf(&b, "\nerrors (newest first):\n%s", strings.Join(errs, ""))
	}
	return b.String()
}

func runStatus(r client.RunRecord) string {
	switch {
	case r.Status == "":
		return "error"
	case r.Status == client.JobFailed && r.ExitCode != nil:
		return fmt.Sprintf("failed(%d)", *r.ExitCode)
	default:
		return string(r.Status)
	}
}

func statusCounts(runs []client.RunRecord) string {
	counts := map[string]int{}
	var order []string
	for _, r := range runs {
		s := strings.SplitN(runStatus(r), "(", 2)[0]
		if counts[s] == 0 {
			order = append(order, s)
		}
		counts[s]++
	}
	slices.Sort(order)
	parts := make([]string, len(order))
	for i, s := range order {
		parts[i] = fmt.Sprintf("%d %s", counts[s], s)
	}
	return strings.Join(parts, ", ")
}

func percentile(runs []client.RunRecord, p float64) string {
	totals := make([]int64, len(runs))
	for i, r := range runs {
		totals[i] = r.TotalMS
	}
	slices.Sort(totals)
	rank := int(p*float64(len(totals))+0.999999) - 1
	return ms(totals[max(0, rank)])
}

func timeSpent(runs []client.RunRecord) string {
	parts := []struct {
		name string
		ms   int64
	}{{name: "sync"}, {name: "queue"}, {name: "copy"}, {name: "run"}, {name: "artifacts"}, {name: "cleanup"}, {name: "other"}}
	var total int64
	var unphased int
	for _, r := range runs {
		if r.PhasesMS == nil && r.JobID != "" {
			unphased++
		}
		got := []int64{r.SyncMS, r.QueueMS, r.PhasesMS["copy"], r.PhasesMS["run"], r.PhasesMS["artifacts"] + r.ArtifactsMS, r.PhasesMS["cleanup"]}
		var known int64
		for i, v := range got {
			parts[i].ms += v
			known += v
		}
		parts[len(parts)-1].ms += max(0, r.TotalMS-known)
		total += r.TotalMS
	}
	if total == 0 {
		return "nothing measured"
	}
	out := make([]string, len(parts))
	for i, p := range parts {
		out[i] = fmt.Sprintf("%s %s (%d%%)", p.name, ms(p.ms), p.ms*100/total)
	}
	if unphased > 0 {
		out = append(out, fmt.Sprintf("%d runs without server phase timings, counted as other", unphased))
	}
	return strings.Join(out, ", ")
}

func ms(v int64) string {
	return (time.Duration(v) * time.Millisecond).Round(100 * time.Millisecond).String()
}
