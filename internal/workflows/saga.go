package workflows

import "go.temporal.io/sdk/workflow"

// Saga accumulates compensation functions and runs them in LIFO order on rollback.
// All compensations are best-effort: errors are logged and execution continues.
type Saga struct {
	compensations []func(workflow.Context)
}

func (s *Saga) AddCompensation(f func(workflow.Context)) {
	s.compensations = append(s.compensations, f)
}

// Compensate runs all registered compensations in LIFO order using a disconnected
// context so they execute even if the parent ctx is cancelled or failed.
func (s *Saga) Compensate(ctx workflow.Context) {
	cctx, _ := workflow.NewDisconnectedContext(ctx)
	logger := workflow.GetLogger(ctx)
	for i := len(s.compensations) - 1; i >= 0; i-- {
		func(f func(workflow.Context)) {
			defer func() {
				if r := recover(); r != nil {
					logger.Error("saga: compensation panicked", "recover", r)
				}
			}()
			f(cctx)
		}(s.compensations[i])
	}
}
