package executor

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os/exec"
	"sync"
	"time"
)

// Line is one line of child output, delivered to a Start callback as it arrives.
type Line struct {
	Text   string
	Stderr bool
}

// Proc is a child started by Start; Wait must be called exactly once to reap it.
type Proc struct {
	cmd    *exec.Cmd
	ctx    context.Context
	name   string
	start  time.Time
	logger *slog.Logger
	mu     sync.Mutex // serialises onLine across the stdout and stderr copiers
	onLine func(Line)
	stdout *lineWriter
	stderr *lineWriter
	out    *ring
	err    *ring
}

// Start launches a command and hands every stdout and stderr line to onLine
// (nil discards) as it arrives, besides keeping the tails Wait returns. onLine
// runs serialised on the pipe copiers, so a slow callback back-pressures the
// child.
func (e *Executor) Start(ctx context.Context, onLine func(Line), name string, args ...string) (*Proc, error) {
	return e.start(ctx, nil, onLine, name, args...)
}

func (e *Executor) start(ctx context.Context, stdin io.Reader, onLine func(Line), name string, args ...string) (*Proc, error) {
	p := &Proc{
		cmd:    e.newCmd(ctx, name, args...),
		ctx:    ctx,
		name:   name,
		start:  time.Now(),
		logger: e.logger,
		onLine: onLine,
		out:    newRing(maxCapturedLines),
		err:    newRing(maxCapturedLines),
	}
	p.stdout = &lineWriter{emit: func(s string) { p.out.push(s); p.deliver(Line{Text: s}) }}
	p.stderr = &lineWriter{emit: func(s string) { p.err.push(s); p.deliver(Line{Text: s, Stderr: true}) }}
	p.cmd.Stdin, p.cmd.Stdout, p.cmd.Stderr = stdin, p.stdout, p.stderr

	if err := p.cmd.Start(); err != nil {
		return nil, preferCtxErr(ctx, name, err)
	}
	e.logger.Debug("exec: started", "cmd", name, "argc", len(args))
	return p, nil
}

func (p *Proc) deliver(l Line) {
	if p.onLine == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.onLine(l)
}

// Wait reaps the child and returns its result; *Result is always non-nil. A
// child cut short by ctx returns ctx's error, not its exit.
func (p *Proc) Wait() (*Result, error) {
	err := p.cmd.Wait()
	p.stdout.flush()
	p.stderr.flush()
	result := &Result{
		Stdout:    p.out.tail(),
		Stderr:    p.err.tail(),
		Duration:  time.Since(p.start),
		Truncated: p.out.dropped,
	}
	retErr := preferCtxErr(p.ctx, p.name, splitExitError(err, result))
	p.logger.Debug("exec: completed", "cmd", p.name, "exit", result.ExitCode, "duration", result.Duration)
	return result, retErr
}

// preferCtxErr swaps err for ctx's error once ctx is done: os/exec reports
// the SIGTERM exit over the watcher's context error, so a cancelled child
// would otherwise read as a plain non-zero exit.
func preferCtxErr(ctx context.Context, name string, err error) error {
	if ctxErr := ctx.Err(); ctxErr != nil {
		return fmt.Errorf("%s: %w", name, ctxErr)
	}
	return err
}
