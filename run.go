package godel

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
)

func (g *Godel) run(ctx context.Context, params *Params) (result *Result, retErr error) {
	claimRun()
	if params == nil || params.Node == nil {
		return nil, errors.New("godel: params and node are required")
	}
	if g == nil {
		return nil, errors.New("godel: nil runner")
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}
	input, err := readTask(ctx)
	if err != nil {
		return nil, err
	}
	p, err := registerProgram(cwd)
	if err != nil {
		return nil, fmt.Errorf("godel: register source: %w", err)
	}
	lock, err := acquireProgramLock(p)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := lock.Close(); err != nil && retErr == nil {
			retErr = fmt.Errorf("godel: close lock: %w", err)
		}
		if retErr != nil {
			result = nil
		}
	}()
	if input == "" {
		input, err = latestFinalResponse(p)
		if err != nil {
			return nil, err
		}
	}
	r, err := newRunRecord(p, input, cwd)
	if err != nil {
		return nil, err
	}
	defer func() {
		// Failed work is saved before improvement.
		// Do not overwrite its original failure
		// with auxiliary errors or repeat that
		// transition.
		if retErr != nil && r.Status != "failed" {
			r.Status = "failed"
			r.Failure = retErr.Error()
			if err := r.save(p); err != nil {
				retErr = errors.Join(retErr, fmt.Errorf("godel: persist failure: %w", err))
			}
		}
	}()
	work, err := executeTree(ctx, params.Node, input)
	r.Sessions = work.Sessions
	r.Invocations = work.Invocations
	r.Stopped = work.Stopped
	r.FinalResponse = work.Result.Text
	var workErr error
	if err != nil {
		workErr = fmt.Errorf("godel: execute: %w", err)
		r.Status = "failed"
		r.Failure = workErr.Error()
	} else {
		r.Status = "completed"
		if work.Stopped {
			r.Status = "stopped"
		}
	}
	if err := r.save(p); err != nil {
		return nil, errors.Join(workErr, err)
	}
	if params.SelfImprove != nil && (workErr == nil || ctx.Err() == nil) {
		improvement, err := improve(ctx, params.SelfImprove, improvementContext{
			Goal:             g.goal,
			ProgramID:        p.ID,
			Source:           p.Source,
			ProgramDirectory: p.Dir,
			RunRecord:        r.path(p),
			Directory:        cwd,
			Input:            input,
			Output:           work.Result.Text,
			Failure:          r.Failure,
			Loop:             params.Loop,
			Stopped:          work.Stopped,
			Sessions:         work.Sessions,
			Invocations:      work.Invocations,
		})
		r.ImprovementInvocations = improvement.Invocations
		r.Sessions = append(r.Sessions, improvement.Sessions...)
		r.ImprovementResponse = improvement.Result.Text
		if err != nil {
			r.ImprovementError = err.Error()
			fmt.Fprintln(&improvementStderr{start: true}, "attempt:", err)
			if workErr != nil {
				workErr = errors.Join(workErr, err)
			}
		}
		if err := r.save(p); err != nil {
			return nil, errors.Join(workErr, err)
		}
	}
	if workErr != nil {
		return nil, workErr
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if !params.Loop || work.Stopped {
		return &work.Result, nil
	}
	return nil, launchFresh(ctx, p.Source, work.Result.Text)
}

// readTask returns explicit stdin unchanged, or
// an empty string to request history fallback.
// Read before taking the program lock since stdin
// may block.
func readTask(ctx context.Context) (string, error) {
	st, err := os.Stdin.Stat()
	if err != nil {
		return "", fmt.Errorf("godel: stat stdin: %w", err)
	}
	// Character devices include terminals and /dev/null.
	if st.Mode()&os.ModeCharDevice != 0 {
		return "", nil
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	b, err := io.ReadAll(os.Stdin)
	if err != nil {
		return "", fmt.Errorf("godel: read stdin: %w", err)
	}
	if strings.TrimSpace(string(b)) == "" {
		return "", nil
	}
	return string(b), nil
}
