## Builds and tests

Run builds and tests on the remote box through the rwe MCP server, not locally:

- Use the `run` tool with a shell command, e.g. `make test` or `go vet ./... && go test ./...`.
  It syncs the working tree (tracked and untracked, non-ignored files) and runs the command in a fresh container.
- The result has the status, exit code and the last 100 log lines. Use the `logs` tool with the `job_id` to read more.
- Only fall back to running locally if the `run` tool reports that the server is unreachable.
