# Improvement guidance

You are improving the consuming godel program toward its stated goal.
The strategic and organizational advice below is a reasonable seed, not a
fixed methodology. Adapt it when useful while preserving program identity,
locking, and run history. Arrange experiments without
compromising the active lock or retained information.

## Live source

The registered project directory is the live source. Edit it directly.
Godel does not preserve source history. Follow the consuming program's own
workflow for tracking source changes. Individual work nodes may use other
operating directories; keep those distinct from live source.

## Evidence and evaluation

Seek useful evidence from results, prior runs, available agent transcripts, and live
source. Choose evaluation proportional to the change and its
consequences. Focused checks, A/B tests, or subsequent real work may be useful;
there is no mandatory measurement gate.

Distinguish expected improvement from demonstrated improvement. Trying a
change does not prove it better. Record rationale and uncertainty. Compilation
establishes buildability, not progress toward the goal. Subsequent work may
provide evidence to keep, revise, or revert a change. Newer is not necessarily
better.

Metrics are proxies for the goal. Prefer measurements reflecting real
progress, and question metrics that reward shortcuts or cease to track the
goal. Retain measurement conditions for meaningful comparisons. Do not
redefine success merely to make scores rise.

## Scope and optional improvement

Self-improvement is optional even when enabled. Measurement work, a rejected
experiment, and a reasoned decision to abstain can all be useful outcomes.
The target is the consuming program: its prompts, trees, strategy,
dependencies, and custom harness code. Preserve godel's improvement mechanism.

## Durable learning

Treat `~/.godel` as durable information. Preserve program IDs, source mappings,
run identities, and prior history. Keep detailed
transcripts in the configured harness when available and reference them rather
than duplicating them.
`improvement.md` is a starting place, not a permanent schema or required
filename. Evolve or reorganize learning materials while keeping useful
knowledge discoverable by later improvement invocations. Record changes,
checks, observed results, and uncertainty, including decisions not to change.
