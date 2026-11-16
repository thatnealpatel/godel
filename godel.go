package godel

import (
	"context"
	_ "embed"
	"fmt"
)

func New(goal string) *Godel { return &Godel{goal: goal} }

// Godel runs nodes in pursuit of a program's goal.
type Godel struct{ goal string }

// Run executes the tree, persists its lifecycle, and optionally
// improves it. Loop replaces this process with a fresh go run .
// using the work output as stdin. Nonblank explicit stdin wins;
// otherwise the latest saved nonblank final_response becomes the
// task for this fresh run, or an error is returned if none is
// available. It panics on a second call in the process or lock
// contention. On all returned errors the result is nil. See the
// package documentation for process protocols.
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

// Node describes a process invocation
// and optional decision continuations.
// Exec names an executable, not a shell
// command. Work instructions belong in
// Args; generic selectors must supply
// their own branch and JSON response
// instructions. Directory is the
// operating directory, not the consuming
// program's source.
type Node struct {
	Directory string
	Exec      string
	Args      []string
	Env       []string
	Branches  []Branch
	stop      bool
}

// Branch describes a selectable continuation.
type Branch struct {
	When string
	Then *Node
}

// Result carries final output and
// lightweight execution metadata. It is
// meaningful only when Run returns a
// nil error.
type Result struct {
	Text    string
	Session string
}

// Decide constructs a Yah selector with the supplied
// continuations. Its prompt includes the indexed branch
// descriptions and JSON response instructions; stdin
// remains the original task unchanged.
func Decide(prompt string, branches ...Branch) *Node {
	prompt += "\n\n" + routingInstructions + "\n"
	for i, branch := range branches {
		prompt += fmt.Sprintf("%d: %s\n", i, branch.When)
	}
	prompt += "\n" + jsonResponseInstructions
	return &Node{
		Exec:     "yah",
		Args:     []string{"-yolo", "-replace", prompt},
		Branches: branches,
	}
}

var (
	//go:embed templates/response.md
	jsonResponseInstructions string

	//go:embed templates/routing.md
	routingInstructions string
)

// Stop constructs a terminal node that ends
// the current tree normally. In the work
// tree it also prevents another loop launch;
// in SelfImprove it ends only improvement.
// When selected, the selector's response
// text becomes the tree's final result.
func Stop() *Node { return &Node{stop: true} }
