// Package godel helps you build goal-directed execution lanes that can revise how they work.
// A program describes its work as a tree of processes: ordinary executables,
// agents, or other godel programs. Decisions choose a path through that tree.
// With optional self-improvement and looping, a run's experience can inform
// source changes that the next run builds and uses.
//
// Start with a single useful command, give different tasks specialized branches,
// or compose programs that develop their own approaches to smaller goals.
// The examples below are starting points to adapt: you choose the goal, the
// work to try, and how the program decides when to stop. The running example
// develops a textually grounded analysis of Wittgenstein's private-language
// argument, considers objections, and revises its interpretation.
//
// A harness is an ordinary subprocess Node: Set Exec to "claude", "pi",
// "codex", or another executable, and Args to its noninteractive CLI arguments.
// Godel sends the task on stdin and uses stdout as the result. Choose text output,
// not a harness's JSON event stream or response envelope. No backend adapter is
// required. Work instructions belong in Args; the goal passed to New is context
// for self-improvement, not a prompt automatically sent to work nodes.
//
// # A first program
//
// This complete program asks Claude Code to analyze Wittgenstein's
// private-language argument. New supplies the goal, Node describes the command,
// and Run executes it and returns its output.
// Neither Yah nor self-improvement is needed.
//
// Put it in main.go in its own module, with a go.mod requiring patel.codes/godel
// (for example, use go mod init example.com/wittgenstein followed by
// go get patel.codes/godel). Launch from that module's live source directory
// with Go and an authenticated claude on PATH. Node execution requires
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
// A decision tree can choose between doing more work and returning an answer.
// This complete program uses Pi to assess the analysis of Wittgenstein:
// Does it need further work on textual support or objections, or is it ready
// to return? The selector chooses between revision and Stop.
// Loop carries the work output into a fresh invocation, giving the program
// another opportunity to examine its answer. SelfImprove lets it also revise
// the live source between runs. These options can be used independently.
//
// Use a separate module as above, with authenticated pi and codex on PATH.
// Pi handles selection and answering; Codex handles self-improvement. The Codex
// flags below bypass approvals and sandboxing so it can edit the live source
// and retained learning materials. Use them only in a trusted, externally
// sandboxed environment, or choose permissions appropriate to your setup. The
// prompts ask for preservation of the original question and a defensible
// interpretation; they do not enforce scholarly accuracy or eventual
// termination. Stop is the selector's judgment that the analysis addresses the
// question, not a claim that a philosophical dispute has been settled.
//
// Within a run, the selector and its selected continuation receive the same
// original input. The selector's text is not passed along as a critique or plan,
// so the answering node has its own revision instructions. Across loop launches,
// the handoff keeps the original question alongside the candidate; you can adapt
// that output format to carry whatever continuing context your program needs.
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
//
//		g := godel.New("Develop a textually grounded analysis of Wittgenstein's private-language argument.")
//		node := &godel.Node{
//			Exec: "pi",
//			Args: []string{
//				"--print", "--mode", "text",
//				"--append-system-prompt", `Input is either an original question or a handoff
//	containing the original question and a candidate analysis of Wittgenstein's
//	private-language argument. Select branch 0 if the analysis is absent, lacks
//	relevant section references to Philosophical Investigations, conflates textual
//	evidence with interpretation, or neglects a serious objection. Select branch 1
//	when it addresses the question with a supported interpretation, considers the
//	objection, and acknowledges remaining uncertainty; put the final analysis in
//	text. You only route: Your reasoning is not passed to the answering node.
//	Return only one JSON object, with no Markdown fences or other text:
//	{"branch":0,"text":""} to answer or revise, or
//	{"branch":1,"text":"the final answer"} to stop.`,
//			},
//			Branches: []godel.Branch{
//				{
//					When: "The Wittgenstein analysis needs textual support or further argument",
//					Then: &godel.Node{
//						Exec: "pi",
//						Args: []string{
//							"--print", "--mode", "text",
//							"--append-system-prompt", `Analyze the original question about Wittgenstein's
//	private-language argument, or revise the supplied candidate. Cite relevant
//	sections of Philosophical Investigations, distinguish textual evidence from
//	interpretation, consider a serious objection, and acknowledge uncertainty.
//	Do not invent quotations.
//	Independently identify any correction needed; you do not receive the selector's
//	critique. Return a handoff containing the ORIGINAL QUESTION verbatim and the
//	CANDIDATE ANSWER. Preserve all original qualifications and constraints.`,
//						},
//					},
//				},
//				{
//					When: "The Wittgenstein analysis is supported and addresses the question and an objection",
//					Then: godel.Stop(),
//				},
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
// A selector must return Godel's JSON object as plain stdout, not Pi's JSON
// event stream. Branch indices are zero-based positions in Branches. For a
// generic Node, When labels are not automatically sent to the process: Keep
// the routing prompt aligned with the branch order. Decide is a Yah-specific
// convenience that builds this prompt; other harnesses use Node directly.
// Codex's "-" reads the generated improvement instructions from stdin, while
// --skip-git-repo-check lets the example run in a module without a Git repository.
//
// # Composing specialists and tools
//
// Branches can offer different ways of tackling the Wittgenstein question.
// You might give close reading of Philosophical Investigations, research into
// competing interpretations, and criticism of the draft their own specialized
// continuations, or use a conventional tool to look up section references.
// A continuation can itself be a decision tree. Nodes invoke executables, so
// existing tools and custom harnesses can sit alongside agents without sharing
// an implementation language.
//
// A Node can also invoke another godel program. Here childSource is the child's
// absolute live-source module directory:
//
//	child := &godel.Node{
//		Directory: childSource,
//		Exec:      "go",
//		Args:      []string{"run", "."},
//	}
//
// Use child as a work node or branch continuation. The child is a separate
// process with its own New/Run, goal, history, and improvement policy. For
// example, a child with SelfImprove enabled and Loop disabled can handle one
// assignment to examine an objection to the private-language argument, improve
// itself, and return its analysis to the parent. A looping child retains control
// until it returns or is stopped. Composition
// happens through subprocess nodes, rather than repeated Run calls in one
// process.
//
// The parent receives the child's stdout. Keep incidental diagnostics on stderr
// and use stdout for the result and any context you want the parent to receive.
// For work spanning multiple loop launches, that output might carry an enduring
// research question, section references, competing readings, or unresolved
// objections. Godel passes the output onward as-is; it does not merge it with
// the original task.
//
// # Evolving the program
//
// Self-improvement can reach beyond prompt edits. The consuming program's live
// source includes its tree, strategy, dependencies, and custom harness code.
// For the Wittgenstein program, you might separate textual research from
// argument criticism, add a section-reference lookup tool, or revise the
// handoff to retain objections that earlier drafts overlooked. These are
// possibilities to explore, not a required design or sequence of improvements.
//
// With SelfImprove configured, the supplied process reviews the run in light of
// the goal and may edit that live source and retained learning materials, or
// leave the source unchanged. Edits affect later launches, not the work just
// completed. The goal supplied to New provides improvement context; put
// instructions for work nodes and selectors in their Args, since the goal is
// not automatically added to their prompts.
//
// Under $HOME/.godel, each registered program retains run history, immutable
// source archives, and learning materials initially seeded as improvement.md.
// A run record links the input, output, failure, session references, and
// invocation observations to its source version. Learning materials can evolve
// in form as useful knowledge accumulates. Prior attempts, old implementations,
// and notes are material for comparing, combining, revisiting, or abandoning
// ideas. Archives are not execution workspaces: apply historical source to the
// live project to use it. History does not resume a process or restore a run's
// identity.
//
// Nodes can include findings, checks, resource reports, or artifact references
// in ordinary output when useful to later work or improvement. Choose a form
// that suits your program; there is no required evidence schema or evaluation
// gate. Such reports come from the nodes; Godel itself records invocation
// timing, errors, selected branch indices, and captured session IDs. A source
// change is an experiment, not proof of improvement. Results and retained
// experience can help you decide what to keep, revise, or try next.
//
// # Reference: runs and input
//
// Construction does not execute work. Call Run exactly once per process
// lifetime; a second call panics, including after an earlier call failed.
// Launch from the consuming program's live-source directory: its canonical
// path identifies the registered program. Individual nodes may use other
// operating directories without changing that source identity.
//
// Supply nonwhitespace, nonterminal stdin and close it to signal EOF, for
// example with printf or a redirected task file. Explicit input wins unchanged.
// For a terminal or character device (including /dev/null), stdin is not read.
// These devices, empty stdin, and whitespace-only stdin instead use this
// registered program's most recent saved nonblank final_response, unchanged.
// History is scanned in descending UUIDv7 .json filename order as a practical
// recency convention, skipping blank responses regardless of run status.
// Missing usable output returns an error, as does an unreadable or malformed
// record encountered during the scan.
//
// A saved handoff, partial output from failed work, or an already-final answer
// can all become input to a fresh run. Every invocation creates a new run ID
// and history record, even when input came from history. The same fallback rule
// applies to a fresh loop launch if the preceding work output was blank.
//
// # Reference: processes and routing
//
// Nodes execute directly, without a shell: Exec names the executable, Args
// holds its arguments, and Directory selects its operating directory. Env
// overrides inherited environment entries. Executable lookup uses the runner's
// PATH, before node environment overrides. Each process receives finite stdin
// and EOF. Its stdout is preserved exactly, and stderr is also sent to the
// runner's stderr. The last valid stderr line of the form "yah-session: ID"
// supplies Result.Session. This Yah marker remains the only session protocol;
// other harnesses without it leave Session empty. Godel does not normalize
// backend session IDs. A failed process returns an error, not a successful
// partial result; on every returned Run error the result is nil.
//
// Within a run, each selector and selected continuation receives the original
// task exactly once on stdin, unchanged, not the preceding selector's output.
// A selector emits one JSON object with "branch" (an in-range zero-based
// integer) and "text" (a string), for example {"branch":0,"text":"done"}.
// Decide is an explicit Yah convenience; it includes indexed branch descriptions
// and response instructions in Yah's prompt. Generic Node selectors receive Args
// verbatim: supply their branch and response instructions there. No routing
// metadata is appended to stdin. Any node-reported evidence must fit this output
// contract, rather than adding diagnostics outside a selector's JSON object.
//
// Responses use standard encoding/json decoding: unknown fields are ignored,
// repeated fields follow the decoder's usual behavior, and omitted or null
// text is empty. A missing, null, mistyped, or out-of-range branch is an error.
// An ordinary leaf's stdout is the final text; for a selected Stop, it is the
// selector's text field instead. Stop ends execution normally even with Loop
// enabled. There is no fixed run limit if the program keeps choosing more work.
//
// # Reference: improvement and fresh launches
//
// SelfImprove is an optional Node tree; nil disables improvement. Choose its
// commands, arguments, environment, and operating directories. For example, a
// Pi process configured for noninteractive text output:
//
//	improver := &godel.Node{
//		Exec: "pi",
//		Args: []string{"--print", "--mode", "text"},
//	}
//
// Or Claude with a deliberate permissions bypass, only when you trust it to
// edit the live source without permission prompts:
//
//	improver := &godel.Node{
//		Exec: "claude",
//		Args: []string{"--print", "--output-format", "text", "--dangerously-skip-permissions"},
//	}
//
// Set Params.SelfImprove to the chosen improver. The complete example uses
// Codex instead. These are configurations, not backend adapters: Callers choose
// suitable noninteractive, permission, and session-retention flags for their
// harness. Install and authenticate the chosen CLI before running the program.
//
// Each improvement node's Directory controls only that process's cwd, not source
// identity, archives, or the improvement target. If omitted, it defaults to the
// registered live source for that node. The caller's nodes are not modified.
// The generated context always identifies the registered live Source
// independently of those operating directories.
//
// Exec, Args, and Env follow the ordinary process contract. Args are passed
// verbatim: Godel adds no Yah-specific flags or prompt arguments. Instead, stdin
// contains the complete improvement instructions and guidance, followed by a
// "# Run context" heading and a JSON object with the goal and existing run
// evidence, including Source, input, output, failure, and invocation history.
// Every selected process receives that same complete stdin, not the preceding
// selector's text. Routing follows the work-tree protocol: selectors emit JSON,
// leaves return their stdout, and a selected Stop returns the selector's text.
// Stop in the improvement tree ends only improvement; it never suppresses an
// outer loop launch. Nodes are checked only when selected, so unselected invalid
// branches are ignored and selected cycles continue until canceled or stopped.
// Improvement stderr is displayed with a "godel: improvement: " line prefix for
// every invocation, while raw diagnostics still supply errors and any yah-session
// marker. History retains the final response, error, all ordered invocation
// observations, and all captured sessions.
//
// Run executes work once and saves its outcome before optional improvement.
// A work-tree Stop prevents another launch but does not prevent an enabled
// improvement attempt. Failed work can also receive improvement if the context
// has not been canceled. It is not retried: Run returns the work error, possibly
// joined with an improvement error, without looping even when Loop is enabled.
//
// After successful work, an improvement-process error is recorded and diagnosed;
// that error alone does not replace the work result or disable looping.
// Persistence errors, context cancellation, lock errors, and launch errors can
// still prevent a successful return or another run.
//
// If Loop is false or the work tree stopped, Run returns the work result. Otherwise
// it replaces the runner with a fresh go run . in the live-source directory,
// using the previous work output as stdin, not the improvement response. Only
// live source is built. Main and setup run again; contexts and other in-memory
// state do not survive. The launch preserves stdout, stderr, environment, and
// command-line arguments. The runner is replaced rather than kept as a waiting
// parent, but the Go tool waits for the program it starts: each loop leaves a
// waiting Go-tool wrapper until the final invocation exits.
//
// # Reference: locking and cancellation
//
// A program holds godel.lock while Run executes. Contention on the program lock
// panics. On a loop launch, exec closes the lock and the next invocation takes
// a fresh nonblocking exclusive lock in Run. A competing invocation can acquire
// the lock in that gap; the next invocation panics if the competitor still
// holds it. No lock descriptor is inherited.
//
// Each work or improvement node runs in a transient user systemd scope.
// Context cancellation sends SIGKILL to the scope, including descendants that
// change process groups. There is no process-group fallback if the user manager
// is unavailable. Waiting for inherited output pipes is bounded. Callers should
// cancel the context on interrupt and termination signals, as in the examples.
// Killing the runner with SIGKILL cannot trigger cancellation and is not
// guaranteed to clean up its scopes. Scopes are not a security boundary against
// processes deliberately moving to other cgroups.
package godel
