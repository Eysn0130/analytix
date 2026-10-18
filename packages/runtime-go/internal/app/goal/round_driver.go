package goal

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"

	"analytix.local/runtime-go/internal/contracts"
	domaingoal "analytix.local/runtime-go/internal/domain/goal"
)

type DriverDependenciesV1 struct {
	ReadGoal func(string) (map[string]any, error)
	Ready    func(string) bool
	Send     func(context.Context, string, *RoundAttemptV1) error
	// Deferred is limited to a rejected idle claim before any primary commit.
	Deferred    func(error) bool
	BlockBudget func(string, domaingoal.RefV1) (map[string]any, error)
	Publish     func(string, *domaingoal.ActivationV1)
}

type liveGoalV1 struct {
	ref     domaingoal.RefV1
	armed   bool
	attempt *RoundAttemptV1
}

// RoundDriverV1 owns activation only. It feeds the existing turn admission;
// it owns no queue, execution policy, model grants or durable activation.
type RoundDriverV1 struct {
	mu      sync.Mutex
	deps    DriverDependenciesV1
	live    map[string]*liveGoalV1
	closed  bool
	workers sync.WaitGroup
}

type RoundAttemptV1 struct {
	driver          *RoundDriverV1
	threadID        string
	source          domaingoal.RoundSourceV1
	prompt          string
	context         context.Context
	cancel          context.CancelFunc
	cancelExecution context.CancelFunc
	committed       bool
	finished        bool
}

func NewRoundDriverV1(deps DriverDependenciesV1) *RoundDriverV1 {
	return &RoundDriverV1{deps: deps, live: map[string]*liveGoalV1{}}
}

func (*RoundAttemptV1) MarshalJSON() ([]byte, error) {
	return nil, errors.New("goal admission is process-local")
}
func (a *RoundAttemptV1) Source() domaingoal.RoundSourceV1 { return a.source }
func (a *RoundAttemptV1) Prompt() string                   { return a.prompt }
func (a *RoundAttemptV1) ThreadID() string                 { return a.threadID }

func (a *RoundAttemptV1) validLocked() bool {
	live := a.driver.live[a.threadID]
	return !a.driver.closed && a.context.Err() == nil && live != nil && live.armed && live.attempt == a &&
		live.ref == (domaingoal.RefV1{ID: a.source.GoalID, Revision: a.source.Revision}) && !a.finished
}

func (a *RoundAttemptV1) Validate(threadID, prompt string) error {
	if a == nil || a.driver == nil {
		return ErrGoalRefConflictV1
	}
	a.driver.mu.Lock()
	defer a.driver.mu.Unlock()
	if threadID != a.threadID || prompt != a.prompt || !a.validLocked() {
		return ErrGoalRefConflictV1
	}
	return nil
}

// WithAdmission serializes host disarm with the exact primary commit. The
// returned bool is the primary witness, including recovery-required writes.
func (a *RoundAttemptV1) WithAdmission(commit func() (bool, error)) error {
	if a == nil || a.driver == nil {
		return ErrGoalRefConflictV1
	}
	a.driver.mu.Lock()
	if !a.validLocked() || a.committed {
		a.driver.mu.Unlock()
		return ErrGoalRefConflictV1
	}
	committed, err := commit()
	a.committed = committed
	a.driver.mu.Unlock()
	return err
}

func (a *RoundAttemptV1) BindExecutionCancel(cancel context.CancelFunc) error {
	a.driver.mu.Lock()
	defer a.driver.mu.Unlock()
	if !a.validLocked() {
		cancel()
		return ErrGoalRefConflictV1
	}
	a.cancelExecution = cancel
	return nil
}

func (d *RoundDriverV1) Activation(threadID string, goal map[string]any) *domaingoal.ActivationV1 {
	ref, _, _, counted := domaingoal.CountedRecordV1(goal)
	if !counted {
		return nil
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	activation := "disarmed"
	if live := d.live[threadID]; !d.closed && live != nil && live.ref == ref && live.armed {
		activation = "armed"
	}
	return &domaingoal.ActivationV1{RefV1: ref, Activation: activation}
}

func (d *RoundDriverV1) Mutate(threadID string, patch map[string]any, commit func() (map[string]any, error)) (map[string]any, error) {
	d.mu.Lock()
	operation := contracts.StringField(patch, "operation")
	live := d.live[threadID]
	if d.closed {
		d.mu.Unlock()
		return nil, errors.New("goal driver is closed")
	}
	if operation == "resume" && live != nil && live.armed {
		if ref, ok := ExpectedRefV1(patch); ok && ref == live.ref {
			d.mu.Unlock()
			return nil, errors.New("goal is already armed")
		}
	}
	goal, err := commit()
	if err != nil {
		if errors.Is(err, ErrGoalCommittedRecoveryV1) || errors.Is(err, ErrGoalCommitUnknownV1) {
			if live != nil {
				live.armed = false
				if live.attempt != nil {
					live.attempt.cancel()
					if live.attempt.cancelExecution != nil {
						live.attempt.cancelExecution()
					}
				}
			}
			d.mu.Unlock()
			d.publish(threadID, goal)
			return goal, err
		}
		d.mu.Unlock()
		return nil, err
	}
	ref, _, _, counted := domaingoal.CountedRecordV1(goal)
	armed := counted && contracts.StringField(goal, "status") == "active" && (operation == "create" || operation == "resume" || (operation == "edit" && live != nil && live.armed))
	if live != nil && live.attempt != nil {
		if operation != "edit" || !live.attempt.committed {
			live.attempt.cancel()
			if live.attempt.cancelExecution != nil {
				live.attempt.cancelExecution()
			}
		}
	}
	if counted {
		nextLive := &liveGoalV1{ref: ref, armed: armed}
		if operation == "edit" && live != nil && live.attempt != nil && live.attempt.committed {
			nextLive.attempt = live.attempt
		}
		d.live[threadID] = nextLive
	} else {
		delete(d.live, threadID)
	}
	d.mu.Unlock()
	d.publish(threadID, goal)
	if armed {
		d.Kick(threadID)
	}
	return goal, nil
}

// Observe never arms. Model completion/blocking and budget changes may revoke
// a live activation; ordinary evidence/counter updates preserve its identity.
func (d *RoundDriverV1) Observe(threadID string, goal map[string]any) {
	d.mu.Lock()
	ref, _, _, counted := domaingoal.CountedRecordV1(goal)
	live := d.live[threadID]
	if live != nil && (!counted || ref != live.ref || contracts.StringField(goal, "status") != "active") {
		live.armed = false
		if live.attempt != nil {
			live.attempt.cancel()
		}
	}
	d.mu.Unlock()
	d.publish(threadID, goal)
}

func (d *RoundDriverV1) publish(threadID string, goal map[string]any) {
	if d.deps.Publish != nil {
		d.deps.Publish(threadID, d.Activation(threadID, goal))
	}
}

func (d *RoundDriverV1) Kick(threadID string) {
	d.mu.Lock()
	live := d.live[threadID]
	if d.closed || live == nil || !live.armed || live.attempt != nil {
		d.mu.Unlock()
		return
	}
	goal, err := d.deps.ReadGoal(threadID)
	ref, maximum, started, counted := domaingoal.CountedRecordV1(goal)
	if err != nil || !counted || ref != live.ref || contracts.StringField(goal, "status") != "active" {
		live.armed = false
		d.mu.Unlock()
		d.publish(threadID, goal)
		return
	}
	if started >= maximum {
		live.armed = false
		d.mu.Unlock()
		if d.deps.BlockBudget != nil {
			if next, err := d.deps.BlockBudget(threadID, ref); err == nil || errors.Is(err, ErrGoalCommittedRecoveryV1) {
				goal = next
			} else {
				goal = nil
			}
		}
		d.publish(threadID, goal)
		return
	}
	if d.deps.Ready != nil && !d.deps.Ready(threadID) {
		d.mu.Unlock()
		return
	}
	objective, _ := json.Marshal(contracts.StringField(goal, "objective"))
	prompt := fmt.Sprintf("<goal_round>\nObjective: %s\nRound: %d/%d\n\nContinue working toward the objective in this same session. Treat current workspace, tool results and durable state as authoritative. Make concrete progress and verify it. Before completion, read the current goal and gather evidence that the whole objective is achieved. Preserve the configured evidence, todo and self-check requirements. If work remains, leave the goal active for the next round. Follow the configured goal-tool policy before reporting a blocker.\n</goal_round>", objective, started+1, maximum)
	ctx, cancel := context.WithCancel(context.Background())
	attempt := &RoundAttemptV1{driver: d, threadID: threadID, source: domaingoal.RoundSourceV1{Kind: "goal", GoalID: ref.ID, Revision: ref.Revision, Round: started + 1}, prompt: prompt, context: ctx, cancel: cancel}
	live.attempt = attempt
	d.workers.Add(1)
	d.mu.Unlock()
	go func() {
		defer d.workers.Done()
		err := d.deps.Send(ctx, threadID, attempt)
		d.mu.Lock()
		current := d.live[threadID]
		deferred := err != nil && !attempt.committed && d.deps.Deferred != nil && d.deps.Deferred(err)
		if current != nil && current.attempt == attempt && err != nil {
			if !deferred {
				current.armed = false
			}
			current.attempt = nil
			attempt.finished = true
			attempt.cancel()
		}
		d.mu.Unlock()
		if err != nil {
			goal, _ := d.deps.ReadGoal(threadID)
			d.publish(threadID, goal)
		}
		// Finished can run before Send releases foreground preparation. Recheck
		// after successful Send, or after a deferred claim releases its slot.
		if deferred || err == nil {
			d.Kick(threadID)
		}
	}()
}

func (d *RoundDriverV1) Finished(threadID string, admission domaingoal.RoundAdmissionV1, interrupted bool) {
	attempt, _ := admission.(*RoundAttemptV1)
	d.mu.Lock()
	live := d.live[threadID]
	if attempt != nil {
		attempt.finished = true
		attempt.cancel()
	}
	if live != nil && live.attempt == attempt && attempt != nil {
		live.attempt = nil
		if interrupted {
			live.armed = false
		}
	}
	if live != nil && attempt == nil && interrupted {
		// Session interruption or a model output cap also stops automatic
		// continuation when the terminal turn originated from an ordinary input.
		live.armed = false
		if live.attempt != nil {
			live.attempt.cancel()
			if live.attempt.cancelExecution != nil {
				live.attempt.cancelExecution()
			}
		}
	}
	d.mu.Unlock()
	goal, _ := d.deps.ReadGoal(threadID)
	d.Observe(threadID, goal)
	d.Kick(threadID)
}

// A known primary admission can settle at a host boundary before the normal
// async execution takes ownership. Release it only after start cleanup, and
// disarm; a denied or failed start is never permission for another round.
func (d *RoundDriverV1) StartSettledWithoutExecutionV1(threadID string, admission domaingoal.RoundAdmissionV1) {
	attempt, ok := admission.(*RoundAttemptV1)
	if !ok || attempt.driver != d {
		return
	}
	d.mu.Lock()
	committed := attempt.committed
	d.mu.Unlock()
	if committed {
		d.Finished(threadID, admission, true)
	}
}

func (d *RoundDriverV1) Close(ctx context.Context) error {
	d.mu.Lock()
	d.closed = true
	for _, live := range d.live {
		live.armed = false
		if live.attempt != nil {
			live.attempt.cancel()
			if live.attempt.cancelExecution != nil {
				live.attempt.cancelExecution()
			}
		}
	}
	d.mu.Unlock()
	done := make(chan struct{})
	go func() { d.workers.Wait(); close(done) }()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
