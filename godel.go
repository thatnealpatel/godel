package godel

import "context"

func New(goal string) *Godel { return &Godel{goal: goal} }

// Godel runs nodes in pursuit of a program's goal.
type Godel struct{ goal string }

// Run executes the tree, persists its lifecycle, and optionally
// improves it. Loop replaces this process with a fresh go run . using
// the work output as stdin. Nonblank explicit stdin wins; otherwise
// the latest saved nonblank final_response becomes the task for
// this fresh run, or an error is returned if none is available. It
// panics on a second call in the process or lock contention. Process
// failures and Next errors fail work execution; on all returned
// errors the result is nil. See the package documentation for process
// and transition contracts.
func (g *Godel) Run(ctx context.Context, p *Params) (*Result, error) {
	return g.run(ctx, p)
}

// Params describes one execution and its
// independent improvement/loop policies.
type Params struct {
	Node *Node

	// SelfImprove selects an improvement
	// tree; nil disables it. Each selected
	// process receives the complete
	// instructions and run context on stdin.
	// Args are passed verbatim. Directory
	// controls only each process's cwd,
	// defaulting to live source. Stop ends
	// only improvement, not Loop.
	SelfImprove *Node
	Loop        bool
}

// Node describes a process invocation and
// an optional transition. Exec names an
// executable, not a shell command. The
// caller supplies harness instructions
// in Args and interprets output in Next.
// Directory is the operating directory,
// not the consuming program's source.
type Node struct {
	Directory string
	Exec      string
	Args      []string
	Env       []string

	// Next is called with the process Result
	// only after a successful invocation. It
	// returns the next node or an execution
	// error. An absent Next or a nil node
	// with nil error is an ordinary leaf.
	// Stop ends the tree normally with
	// the current Result unchanged. Every
	// continuation receives the original
	// stdin, not this Result.
	Next func(Result) (*Node, error)
	stop bool
}

// Result carries process stdout. Next
// receives a successful invocation's
// Result. Run returns the final work
// Result only on a nil error.
type Result struct {
	Text string
}

// Stop constructs a terminal node that ends
// the current tree normally. In the work
// tree it also prevents another loop launch;
// in SelfImprove it ends only improvement.
// When returned by Next, it preserves the
// current process Result unchanged. A Stop
// root invokes no process and returns an
// empty Result.
func Stop() *Node { return &Node{stop: true} }
