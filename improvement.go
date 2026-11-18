package godel

import (
	"context"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
)

func improve(ctx context.Context, n *Node, info improvementContext) (execution, error) {
	input, err := json.Marshal(info)
	if err != nil {
		return execution{}, fmt.Errorf("encode improvement context: %w", err)
	}
	stdin := improvementPrompt + "\n\n# Run context\n\n" + string(input)
	result, err := executeTreeWith(ctx, n, stdin, func(ctx context.Context, n *Node, input string) (Result, metadata, error) {
		invocation := *n
		if invocation.Directory == "" {
			invocation.Directory = info.Source
		}
		return invokeWithStderr(ctx, &invocation, input, &improvementStderr{start: true})
	})
	if err != nil {
		return result, fmt.Errorf("improvement attempt: %w", err)
	}
	return result, nil
}

type improvementContext struct {
	Goal, ProgramID, Source, ProgramDirectory, RunRecord string
	Directory, Input, Output                             string
	Failure                                              string
	Loop, Stopped                                        bool
	Sessions                                             []string
	Invocations                                          []metadata
}

var (
	improvementPrompt = improvementInstructions + improvementRider

	//go:embed templates/improvement.md
	improvementInstructions string

	//go:embed templates/rider.md
	improvementRider string
)

// improvementStderr prefixes each diagnostic
// line so a composing runner cannot mistake
// an auxiliary improvement session marker
// for the work session. invokeWithStderr
// still captures the raw bytes for this
// attempt's session and errors.
type improvementStderr struct{ start bool }

func (w *improvementStderr) Write(p []byte) (int, error) {
	written := 0
	for len(p) != 0 {
		if w.start {
			if _, err := io.WriteString(os.Stderr, "godel: improvement: "); err != nil {
				return written, err
			}
			w.start = false
		}
		part := p
		if i := strings.IndexByte(string(p), '\n'); i >= 0 {
			part = p[:i+1]
		}
		n, err := os.Stderr.Write(part)
		written += n
		if err != nil {
			return written, err
		}
		w.start = part[len(part)-1] == '\n'
		p = p[len(part):]
	}
	return written, nil
}
