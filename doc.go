// Package godel runs trees of commands, agents, and other `godel` programs.
// A program chooses a path through its tree to do the work. It can also
// revise its live source and start a fresh run with those changes.
// Self-improvement and looping are optional.
//
// This package aims to describe, not prescribe. Callers are free to build
// whatever abstractions they want. Examples below are illustrative only
// and do not begin to scratch the surface of the types of `godel` programs
// that can be written.
//
// A harness is just a subprocess. Set [Node.Exec] to "claude", "pi", "codex",
// or another executable, and Args to its noninteractive CLI arguments.
// `godel` sends the task on stdin and takes the result from stdout. Choose the
// harness output mode your program wants to interpret. There is no backend
// adapter to write; the examples use text output.
//
// Put work instructions in Args. The goal passed to [New] gives context to
// self-improvement; it is not automatically sent to work nodes.
//
// Start with one useful command. Add transitions for specialized tasks, or invoke
// another `godel` program to work on a smaller goal. You choose the goal, what
// work to try, and when the program should stop. The examples below develop a
// textually grounded analysis of Wittgenstein's private-language argument,
// consider objections, and revise the interpretation.
//
// # A first program
//
// This program asks Claude Code to analyze Wittgenstein's private-language
// argument. [New] supplies the goal, [Node] describes the command, and
// [Godel.Run] executes it and returns its output.
//
// Put the program in main.go in its own module. Create the module with
// go mod init example.com/wittgenstein, then add the dependency with
// go get patel.codes/godel@latest. Run it from that module's live-source directory
// with Go and an authenticated claude on PATH. Node execution also requires
// systemd-run, systemctl, and a working user systemd manager.
//
//	package main
//
//	import (
//		"context"
//		"fmt"
//		"log"
//		"os"
//		"os/signal"
//		"syscall"
//
//		"patel.codes/godel"
//	)
//
//	func main() {
//		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
//		defer stop()
//		g := godel.New("Develop a textually grounded analysis of Wittgenstein's private-language argument.")
//		result, err := g.Run(ctx, &godel.Params{
//			Node: &godel.Node{
//				Exec: "claude",
//				Args: []string{
//					"--print", "--output-format", "text",
//					"--append-system-prompt", `Analyze the question about Wittgenstein's
//	private-language argument. Cite relevant sections of Philosophical Investigations,
//	distinguish textual evidence from interpretation, and consider a serious
//	objection. Acknowledge uncertainty rather than inventing quotations.`,
//				},
//			},
//		})
//		if err != nil {
//			log.Fatal(err)
//		}
//		fmt.Print(result.Text)
//	}
//
// From the example's module root, run:
//
//	printf '%s\n' "Does Wittgenstein rule out a private language, or only private criteria of correctness?" | go run .
//
// # Answer, reconsider, continue
//
// A decision tree lets the program choose whether to do more work or return
// an answer. This program uses Pi to assess the Wittgenstein analysis: Does it
// need better textual support or more work on objections? The caller interprets
// its response in [Node.Next] and chooses revision or [Stop].
//
// Loop passes the work output to a fresh invocation so the program can examine
// its answer again. SelfImprove lets it revise the live source between runs.
// You can use either option without the other.
//
// Use a separate module as above, with authenticated pi and codex on PATH.
// Pi assesses and answers; Codex handles self-improvement. The improvement
// process can edit the live source and retained learning materials.
//
// The prompts ask the program to preserve the original question and give a
// defensible interpretation. They do not enforce scholarly accuracy or eventual
// termination. Stop is the program's judgment that the analysis addresses the
// question. It does not settle the philosophical dispute.
//
// Within a run, the assessment and its continuation get the same original
// input. The assessment's output is not passed on as a critique or plan, so the
// answering node has its own revision instructions. Across loop launches, the
// example's handoff keeps the original question alongside the candidate. Adapt
// that output format to carry the context your program needs. The final result
// is the complete decision JSON, including the question and answer, unchanged.
//
//	package main
//
//	import (
//		"context"
//		"encoding/json"
//		"fmt"
//		"log"
//		"os"
//		"os/signal"
//		"strings"
//		"syscall"
//
//		"patel.codes/godel"
//	)
//
//	// parseDecision interprets this program's response format, not a godel protocol.
//	func parseDecision(text string) (string, error) {
//		var decision struct {
//			Action   string `json:"action"`
//			Question string `json:"question"`
//			Answer   string `json:"answer"`
//		}
//		if err := json.Unmarshal([]byte(text), &decision); err != nil {
//			return "", fmt.Errorf("invalid Wittgenstein decision: %w", err)
//		}
//		if strings.TrimSpace(decision.Question) == "" {
//			return "", fmt.Errorf("decision must preserve the original question")
//		}
//		switch decision.Action {
//		case "revise":
//			return decision.Action, nil
//		case "done":
//			if strings.TrimSpace(decision.Answer) == "" {
//				return "", fmt.Errorf("done decision requires an answer")
//			}
//			return decision.Action, nil
//		default:
//			return "", fmt.Errorf("unknown decision action %q", decision.Action)
//		}
//	}
//
//	func main() {
//		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
//		defer stop()
//
//		g := godel.New("Develop a textually grounded analysis of Wittgenstein's private-language argument.")
//		revise := &godel.Node{
//			Exec: "pi",
//			Args: []string{
//				"--print", "--mode", "text",
//				"--append-system-prompt", `Analyze the original question about Wittgenstein's
//	private-language argument, or revise the supplied candidate. Cite relevant
//	sections of Philosophical Investigations, distinguish textual evidence from
//	interpretation, consider a serious objection, and acknowledge uncertainty.
//	Do not invent quotations.
//	Independently identify any correction needed; you do not receive the assessment's
//	critique. Return only a JSON handoff with "question" (the ORIGINAL QUESTION
//	verbatim, preserving all qualifications and constraints) and "answer" (the
//	CANDIDATE ANSWER).`,
//			},
//		}
//		node := &godel.Node{
//			Exec: "pi",
//			Args: []string{
//				"--print", "--mode", "text",
//				"--append-system-prompt", `Input is either an original question or a JSON
//	handoff containing "question" and "answer" about Wittgenstein's private-language
//	argument. Assess the analysis. Choose "revise" if it is absent, lacks relevant
//	section references to Philosophical Investigations, conflates textual evidence
//	with interpretation, or neglects a serious objection. Choose "done" when it
//	addresses the question with a supported interpretation, considers the objection,
//	and acknowledges remaining uncertainty. You only assess: Your critique is not
//	passed to the answering node.
//	Return only one JSON object, with no Markdown fences or other text:
//	{"action":"revise","question":"the original question verbatim","answer":"the candidate, if any"}
//	or {"action":"done","question":"the original question verbatim","answer":"the final analysis"}.
//	Preserve all original qualifications and constraints.`,
//			},
//			Next: func(result godel.Result) (*godel.Node, error) {
//				action, err := parseDecision(result.Text)
//				if err != nil {
//					return nil, err
//				}
//				if action == "revise" {
//					return revise, nil
//				}
//				return godel.Stop(), nil
//			},
//		}
//		result, err := g.Run(ctx, &godel.Params{
//			Node: node,
//			SelfImprove: &godel.Node{
//				Exec: "codex",
//				Args: []string{
//					"exec", "--skip-git-repo-check",
//					"--dangerously-bypass-approvals-and-sandbox", "-",
//				},
//			},
//			Loop: true,
//		})
//		if err != nil {
//			log.Fatal(err)
//		}
//		fmt.Print(result.Text)
//	}
//
// From this example's module root, run:
//
//	printf '%s\n' "Does Wittgenstein rule out a private language, or only private criteria of correctness?" | go run .
//
// # Making a decision node
//
// A [Node] can set Next to choose a continuation from its successful [Result].
// The consuming Go program owns the harness prompt and output interpretation.
// The library owns process execution, traversal, and history. It does not parse
// decision output or add response instructions to the process's arguments.
//
// A useful decision has a concrete question and continuations that act on it.
// In this example, Pi examines textual support, distinctions between claims,
// and an objection. The caller's parseDecision uses encoding/json to interpret
// its chosen response format and rejects invalid responses. Next returns the
// revision node for gaps, or [Stop] for a supported analysis. These criteria
// and this JSON schema belong to the example, not to `godel`.
//
// The revision node receives the original input, not the assessment's reasoning.
// It has its own instructions for finding and addressing gaps. At Stop, the
// assessment's entire Result is retained, including the JSON text and session.
// Next does not extract an answer or rewrite the result. Other programs can
// interpret plain text, structured output, or any other process response.
//
// # Composing specialists and tools
//
// Transitions can give the Wittgenstein program several ways to approach the
// question. You might give close reading of Philosophical Investigations,
// research into competing interpretations, and criticism of the draft their
// own continuations. Or use a conventional tool to look up section references.
// A continuation can itself be a decision tree.
//
// Nodes invoke executables. Existing tools and custom harnesses can sit
// alongside agents without sharing an implementation language. A [Node] can
// also invoke another `godel` program. Here childSource is the child's absolute
// live-source module directory:
//
//	child := &godel.Node{
//		Directory: childSource,
//		Exec:      "go",
//		Args:      []string{"run", "."},
//	}
//
// Use child as a work node or continuation. It runs in a separate
// process with its own New/Run, goal, history, and improvement policy.
// For example, a child with SelfImprove enabled and Loop disabled can examine
// an objection to the private-language argument, improve itself, and return
// its analysis to the parent. A looping child keeps control until it returns
// or is stopped. Compose programs through subprocess nodes, not repeated
// Run calls in one process.
//
// The parent receives the child's stdout. Put incidental diagnostics on stderr.
// Use stdout for the result and any context the parent needs. Across loop
// launches, that might include the research question, section references,
// competing readings, or unresolved objections. `godel` passes the output on
// unchanged. It does not merge it with the original task.
//
// # Evolving the program
//
// Self-improvement is not limited to prompt edits. The consuming program's
// live source includes its tree, strategy, dependencies, and custom harness
// code. For the Wittgenstein program, you might separate textual research from
// argument criticism, add a section-reference lookup tool, or change the handoff
// to retain objections that earlier drafts overlooked. These are things to try,
// not a required design or sequence.
//
// With SelfImprove configured, the supplied process reviews the run against
// the goal. It may edit the live source and retained learning materials, or
// leave the source alone. Edits affect later launches, not the completed work.
// The goal passed to New is improvement context. Work and decision nodes need
// their instructions in Args; the goal is not added to their prompts.
//
// Each registered program keeps run records and learning materials under
// $HOME/.godel. Learning materials start as improvement.md and can change form
// as useful knowledge accumulates. Run records describe the work, not the
// source that produced it. `godel` does not copy or version the live source.
// If you want to compare or undo edits, preserve the source before the run
// using your own version control or backup workflow.
//
// Nodes can include findings, checks, resource reports, or artifact references
// in their output when useful to later work or improvement. Choose a form that
// fits the program. There is no required evidence schema or evaluation gate.
// These reports come from the nodes. `godel` itself records invocation timing,
// errors, and captured session IDs.
//
// A source change is an experiment, not proof of improvement. Use results and
// retained experience to decide what to keep, revise, or try next.
//
// # Reference: runs and input
//
// Construction does not execute work. Call [Godel.Run] exactly once per process
// lifetime. A second call panics, even if the first call failed.
//
// Launch from the consuming program's live-source directory. Its canonical path
// identifies the registered program. A node can use another operating directory
// without changing that source identity.
//
// To supply a task, pipe nonwhitespace input or redirect a task file to stdin.
// Close stdin to signal EOF. Explicit nonblank input from nonterminal stdin
// wins and is passed through unchanged. Terminals and other character devices,
// including /dev/null, are not read.
//
// For these devices, empty stdin, or whitespace-only stdin, `godel` uses the
// registered program's most recent saved nonblank final_response, unchanged.
// It scans history in descending UUIDv7 .json filename order as a practical
// recency convention. It skips blank responses regardless of run status.
// If no usable output is found, Run returns an error. An unreadable or
// malformed record encountered during the scan is also an error.
//
// A saved handoff, partial output from failed work, or an already-final answer
// can all become input to a fresh run. Each invocation gets a new run ID and
// history record, even when its input came from history. A fresh loop launch
// uses the same fallback rule if the preceding work output was blank.
//
// # Reference: processes and transitions
//
// Nodes execute directly, without a shell. Exec names the executable, Args
// holds its arguments, and Directory selects its operating directory. Env
// overrides inherited environment entries. Executable lookup uses the runner's
// PATH before applying node environment overrides.
//
// Each process receives finite stdin and EOF. Its stdout is preserved exactly.
// Its stderr is also sent to the runner's stderr. The last valid stderr line
// of the form "yah-session: ID" supplies [Result.Session]. This Yah marker is
// the only session protocol. Other harnesses without it leave Session empty;
// `godel` does not normalize backend session IDs.
//
// A failed process returns an error, not a successful partial result.
// On every returned Run error, the result is nil.
//
// Within a run, every continuation receives the original task exactly once on
// stdin, unchanged. It does not receive the preceding process's output. Args
// are passed verbatim; the caller supplies all harness and output instructions.
//
// After a successful process invocation, Next receives its Result by value,
// including Text and Session. A missing Next or a nil continuation with nil
// error is an ordinary leaf: the current Result becomes the tree's result.
// Next may return another node to continue, or an error to fail execution.
// Callback errors are retained in the current invocation's observation; partial
// output and sessions remain in history. Next is not called after process failure.
//
// Returning [Stop] ends the tree normally with the current Result unchanged.
// Unlike an ordinary leaf, Stop in the work tree also prevents a loop launch.
// A Stop root invokes no process and yields an empty Result. Nodes are validated
// only when reached; cycles continue until canceled or stopped. There is no
// fixed run limit if the program keeps choosing more work.
//
// # Reference: improvement
//
// SelfImprove is an optional [Node] tree. Nil disables improvement. Choose its
// commands, arguments, environment, and operating directories. For example,
// use Pi with noninteractive text output:
//
//	improver := &godel.Node{
//		Exec: "pi",
//		Args: []string{"--print", "--mode", "text"},
//	}
//
// Or Claude with noninteractive text output and permission prompts disabled:
//
//	improver := &godel.Node{
//		Exec: "claude",
//		Args: []string{"--print", "--output-format", "text", "--dangerously-skip-permissions"},
//	}
//
// Set [Params.SelfImprove] to the chosen improver. The complete example above
// uses Codex. These are process configurations, not backend adapters.
//
// Each improvement node's Directory controls only that process's cwd. It does
// not change source identity or the improvement target. If omitted,
// Directory defaults to the registered live source for that node. The caller's
// nodes are not modified. The generated context always identifies the
// registered live Source separately from those operating directories.
//
// Exec, Args, and Env follow the ordinary process contract. Args are passed
// verbatim. The library adds no flags or prompt arguments. Instead,
// stdin contains the complete improvement instructions and guidance, followed
// by a "# Run context" heading and a JSON object. The object contains the goal
// and existing run evidence, including Source, input, output, failure, and
// invocation history.
//
// Every continuation receives that same complete stdin, not the preceding
// process's output. Next follows the same contract as in the work tree. A leaf
// or returned Stop retains the current Result unchanged. Stop in the improvement
// tree ends only improvement; it never suppresses an outer loop launch.
//
// Improvement stderr is displayed with a "godel: improvement: " prefix on each
// line, for every invocation. Raw diagnostics still supply errors and any
// yah-session marker. History retains the final response, error, all ordered
// invocation observations, and all captured sessions.
//
// # Reference: work outcomes and loops
//
// Run executes work once and saves the outcome before optional improvement.
// A work-tree Stop prevents another launch, not an enabled improvement
// attempt. Failed work can also receive improvement if the context has not
// been canceled. It is not retried. Run returns the work error, possibly joined
// with an improvement error, without looping even when Loop is enabled.
//
// After successful work, an improvement execution error is recorded and diagnosed.
// That error alone does not replace the work result or disable looping.
// Persistence errors, context cancellation, lock errors, and launch errors can
// still prevent a successful return or another run.
//
// If Loop is false or the work tree stopped, Run returns the work result.
// Otherwise, it replaces the runner with a fresh go run . in the live-source
// directory. The previous work output becomes stdin, not the improvement
// response. Only live source is built.
//
// Main and setup run again. Contexts and other in-memory state do not survive.
// The launch preserves stdout, stderr, environment, and command-line arguments.
// The runner is replaced, not kept as a waiting parent. However, the Go tool
// waits for the program it starts. Each loop leaves a waiting Go-tool wrapper
// until the final invocation exits.
//
// # Reference: locking and cancellation
//
// A program holds godel.lock while Run executes. Contention on this lock
// panics. On a loop launch, exec closes the lock and the next invocation takes
// a fresh nonblocking exclusive lock in Run. Another invocation can acquire
// the lock in that gap. The next invocation panics if the competitor still
// holds it. No lock descriptor is inherited.
//
// Each work or improvement node runs in a transient user systemd scope.
// Context cancellation sends SIGKILL to the scope, including descendants that
// change process groups. There is no process-group fallback if the user manager
// is unavailable. Waiting for inherited output pipes is bounded.
//
// Callers should cancel the context on interrupt and termination signals,
// as the examples do. Killing the runner with SIGKILL cannot trigger
// cancellation and is not guaranteed to clean up its scopes.
//
// # Security
//
// This package provides absolutely no security properties: Callers are
// expected bring their own solution for running untrusted agents using
// trusted harnesses.
package godel
