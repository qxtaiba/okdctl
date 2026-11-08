package wizard

import (
	"context"

	tea "charm.land/bubbletea/v2"
)

type visitResult struct {
	generation uint64
	step       StepID
	message    tea.Msg
}

type visitContextSetter interface{ SetVisitContext(context.Context) }

func (m *Model) beginVisit() {
	m.stopVisit()
	m.generation++
	parent := m.flowContext
	if parent == nil {
		// Standalone model construction has no caller context; RunFlow supplies one.
		parent = context.Background()
	}
	ctx, cancel := context.WithCancel(parent)
	m.cancelVisit = cancel
	if step, ok := m.CurrentStep().(visitContextSetter); ok {
		step.SetVisitContext(ctx)
	}
}

func (m *Model) stopVisit() {
	if m.cancelVisit != nil {
		m.cancelVisit()
		m.cancelVisit = nil
	}
}

func (m *Model) ownCommand(cmd tea.Cmd) tea.Cmd {
	if cmd == nil || m.CurrentStep() == nil {
		return cmd
	}
	return m.scopeCommand(cmd, m.generation, m.CurrentStep().ID())
}

func (m *Model) scopeCommand(cmd tea.Cmd, generation uint64, step StepID) tea.Cmd {
	if cmd == nil {
		return nil
	}
	return func() tea.Msg {
		m.workerMu.Lock()
		if m.closing {
			m.workerMu.Unlock()
			return nil
		}
		m.workers.Add(1)
		m.workerMu.Unlock()
		defer m.workers.Done()
		message := cmd()
		if batch, ok := message.(tea.BatchMsg); ok {
			scoped := make([]tea.Cmd, len(batch))
			for i, child := range batch {
				scoped[i] = m.scopeCommand(child, generation, step)
			}
			return tea.BatchMsg(scoped)
		}
		return visitResult{generation: generation, step: step, message: message}
	}
}

func (m *Model) shutdown() {
	m.workerMu.Lock()
	m.closing = true
	m.workerMu.Unlock()
	m.stopVisit()
	for _, step := range m.steps {
		if stoppable, ok := step.(interface{ Stop() }); ok {
			stoppable.Stop()
		}
	}
	// Commands inherit cancellation; cleanup waits are bounded by their backend deadlines.
	m.workers.Wait()
	for _, step := range m.steps {
		if releaser, ok := step.(interface{ Release() }); ok {
			releaser.Release()
		}
	}
}
