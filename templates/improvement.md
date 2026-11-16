# Improvement task

Assess the consuming program's work, whether completed or failed, in pursuit
of its goal. Inspect the evidence and learning materials, then make a focused change to
its registered live source when useful. You may also leave it unchanged.
Keep useful learning discoverable in the program directory supplied in the
task context.

## Program directory and evidence

Under the supplied `ProgramDirectory` (in `~/.godel`), the layout is:

- `improvement.md`: persistent learning notes, an initial convention that may evolve.
- `runs/<run-id>.json`: run history; `RunRecord` gives the current run's JSON path.
- `src/<version>/`: retained source snapshots; a record's `version` links it to its snapshot.
- `godel.lock`: the active-run lock; do not manipulate it.

Read prior records as evidence, never resume or activate them. In each record:

- `input`, `final_response`, `failure`, and `sessions` hold work evidence
  (`sessions` may also include auxiliary improvement sessions).
- `improvement_response` and `improvement_error` separately record auxiliary
  improvement results.
- `invocations` retains work observations in execution order; the optional
  `improvement_invocations` separately retains improvement observations in
  execution order. Each includes elapsed duration (`elapsed_ns`), error, and a
  nullable selected branch index. `stopped` records work stopping, including a
  root Stop with no process. Older records may lack these observations.

## Task input and live source

The JSON run context at the end of stdin contains:

- A nonempty `Failure` field is the original work error.
- `Output` may contain partial stdout from failed work.
- `Sessions` references the captured work sessions.
- `Invocations` is a small runner record: elapsed duration (`elapsed_ns`), error,
  and nullable selected branch index.

Use these records alongside outputs, errors, and session references to understand
what happened. Failed partial output is evidence of the attempt, not a successful
result. Diagnose and repair the source when useful.

Its `Source` field is the registered live source and your direct editing target.
The process operating directory may differ; it does not change the target.
Use absolute paths under `Source`, or explicitly `cd` there for shell commands.
If your harness uses an ephemeral workspace, that workspace is not the consuming
program. In Yah, direct edits to `Source` are permitted without `tool_campaign`;
do not use campaign workspaces to stage or activate these edits.

Do not modify retained archives or run records.
Archives are information, not execution workspaces. To reuse historical ideas,
apply the desired changes to live source; there is no version-selection or
activation protocol. For a leaf, return a summary of your work and evidence.
If this process is a selector with configured branches, instead follow its
routing instructions and return the required selection JSON. Selected
continuations receive these complete instructions and run context unchanged,
not the selector's text. Stop ends improvement only; it does not change the
work's stop decision or the outer loop policy.

## Run lifecycle

This invocation has already attempted its work.

- If `Failure` is nonempty, the runner returns the original work error after
  improvement, without retrying or launching another run, even when `Loop` is true.
- Otherwise, if `Loop` is true and `Stopped` is false, the runner next launches a
  fresh `go run .` from live source with this work's output as stdin, never your
  improvement response.
- Otherwise it finishes without another launch.

A fresh launch reruns `main` and setup and creates a new run ID and history
record. Run records are history only, not resumable state.
Do not launch the next run yourself.

