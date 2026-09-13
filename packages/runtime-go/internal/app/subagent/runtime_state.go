package subagent

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	effectgateapp "analytix.local/runtime-go/internal/app/effectgate"
	domainjob "analytix.local/runtime-go/internal/domain/job"
	domainsecurity "analytix.local/runtime-go/internal/domain/security"
)

type backgroundJobControl struct {
	cancel         context.CancelFunc
	binding        *domainjob.SecurityBinding
	startBarrier   *BackgroundJobStartBarrier
	done           chan struct{}
	once           sync.Once
	mu             sync.Mutex
	pauseRequested bool
	paused         bool
	pauseRequestID string
	resumeCh       chan struct{}
	requestedAt    time.Time
	pausedAt       time.Time
}

var ErrBackgroundJobStopTimeout = errors.New("background job did not stop before cancellation deadline")

type RuntimeState struct {
	mu                            sync.Mutex
	backgroundJobs                map[string]*backgroundJobControl
	backgroundClosing             bool
	admissionClosed               chan struct{}
	currentByThread               map[string]domainsecurity.TurnSecurityContext
	currentByWorkspace            map[string]domainsecurity.TurnSecurityContext
	transitionThreads             map[string]bool
	transitionWorkspaces          map[string]bool
	delegatedWorkspaceTransitions map[string]<-chan struct{}
	effectGate                    *effectgateapp.Gate
	outputCursors                 map[string]int
	active                        int
	waiters                       []chan struct{}
}

func NewRuntimeState() *RuntimeState {
	return &RuntimeState{
		backgroundJobs:       map[string]*backgroundJobControl{},
		admissionClosed:      make(chan struct{}),
		currentByThread:      map[string]domainsecurity.TurnSecurityContext{},
		currentByWorkspace:   map[string]domainsecurity.TurnSecurityContext{},
		transitionThreads:    map[string]bool{},
		transitionWorkspaces: map[string]bool{},
		effectGate:           effectgateapp.New(),
		outputCursors:        map[string]int{},
	}
}

func (s *RuntimeState) EffectGate() *effectgateapp.Gate {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.effectGate == nil {
		s.effectGate = effectgateapp.New()
	}
	return s.effectGate
}

func (s *RuntimeState) AcquireContextEffect(ctx context.Context, securityContext domainsecurity.TurnSecurityContext) (context.Context, func(), error) {
	gate := s.EffectGate()
	if gate == nil {
		return ctx, nil, errors.New("security context effect gate is unavailable")
	}
	return gate.AcquireEffect(ctx, securityContext)
}

func (s *RuntimeState) AcquireOrdinaryContextEffect(ctx context.Context, securityContext domainsecurity.TurnSecurityContext) (context.Context, func(), error) {
	gate := s.EffectGate()
	if gate == nil {
		return ctx, nil, errors.New("security context effect gate is unavailable")
	}
	return gate.AcquireOrdinaryEffect(ctx, securityContext)
}

func (s *RuntimeState) AcquireContextEffectForAuthority(
	ctx context.Context,
	securityContext domainsecurity.TurnSecurityContext,
	caseDataEffect bool,
) (context.Context, func(), error) {
	if caseDataEffect {
		return s.AcquireContextEffect(ctx, securityContext)
	}
	return s.AcquireOrdinaryContextEffect(ctx, securityContext)
}

func (s *RuntimeState) AcquireSecurityScopeRead(
	ctx context.Context,
	threadID, workspaceRealPath, tenantID, userID string,
) (func(), error) {
	gate := s.EffectGate()
	if gate == nil {
		return nil, errors.New("security context effect gate is unavailable")
	}
	return gate.AcquireTransitionScopeRead(ctx, effectgateapp.TransitionScope{
		ThreadID: threadID, WorkspaceRealPath: workspaceRealPath, TenantID: tenantID, UserID: userID,
	})
}

func (s *RuntimeState) RegisterBackgroundJob(id string, cancel context.CancelFunc) {
	_, _ = s.RegisterBackgroundJobWithStartBarrier(id, cancel)
}

func (s *RuntimeState) RegisterBackgroundJobWithStartBarrier(id string, cancel context.CancelFunc) (*BackgroundJobStartBarrier, bool) {
	return s.registerBackgroundJob(nil, id, nil, cancel)
}

// RegisterBoundBackgroundJob registers cancellation before any external job
// effect begins. It rejects a late registration when a newer case authority
// has already been observed for the parent thread or shared workspace.
func (s *RuntimeState) RegisterBoundBackgroundJob(id string, binding *domainjob.SecurityBinding, cancel context.CancelFunc) bool {
	_, registered := s.RegisterBoundBackgroundJobWithStartBarrier(id, binding, cancel)
	return registered
}

func (s *RuntimeState) RegisterBoundBackgroundJobWithStartBarrier(id string, binding *domainjob.SecurityBinding, cancel context.CancelFunc) (*BackgroundJobStartBarrier, bool) {
	if domainjob.ValidateSecurityBinding(binding) != nil {
		if cancel != nil {
			cancel()
		}
		return nil, false
	}
	return s.registerBackgroundJob(nil, id, binding, cancel)
}

func (s *RuntimeState) registerBackgroundJob(waitContext context.Context, id string, binding *domainjob.SecurityBinding, cancel context.CancelFunc) (*BackgroundJobStartBarrier, bool) {
	if cancel == nil {
		return nil, false
	}
	if s == nil {
		cancel()
		return nil, false
	}
	id = strings.TrimSpace(id)
	if id == "" {
		cancel()
		return nil, false
	}
	startBarrier := NewBackgroundJobStartBarrier()
	for {
		s.mu.Lock()
		if s.backgroundJobs == nil {
			s.backgroundJobs = map[string]*backgroundJobControl{}
		}
		if s.admissionClosed == nil {
			s.admissionClosed = make(chan struct{})
		}
		if s.backgroundClosing || s.backgroundJobs[id] != nil || binding != nil && !s.bindingAllowedLocked(binding) || waitContext != nil && waitContext.Err() != nil {
			s.mu.Unlock()
			cancel()
			return nil, false
		}
		if binding != nil && s.bindingTransitioningLocked(binding) {
			threadKey := securityAuthorityMapKey("thread", binding.TenantID, binding.UserID, binding.ParentThreadID)
			workspaceKey := securityAuthorityMapKey("workspace", binding.TenantID, binding.UserID, binding.ParentWorkspaceRealPath)
			settled := s.delegatedWorkspaceTransitions[workspaceKey]
			admissionClosed := s.admissionClosed
			// A sibling's short child-context commit does not revoke this
			// parent. Wait only for that delegated transition, then recheck
			// the exact binding under the same lock used for registration.
			// Parent and ordinary workspace transitions remain fail-closed.
			if waitContext == nil || settled == nil || s.transitionThreads[threadKey] {
				s.mu.Unlock()
				cancel()
				return nil, false
			}
			s.mu.Unlock()
			select {
			case <-settled:
			case <-admissionClosed:
				cancel()
				return nil, false
			case <-waitContext.Done():
				cancel()
				return nil, false
			}
			continue
		}
		s.backgroundJobs[id] = &backgroundJobControl{
			cancel: cancel, binding: domainjob.CloneSecurityBinding(binding), startBarrier: startBarrier, done: make(chan struct{}),
		}
		s.mu.Unlock()
		return startBarrier, true
	}
}

// ObserveSecurityContextAndCancelInvalidatedJobs is the atomic convenience
// path used by non-server callers and tests. Production turn start holds the
// returned two-phase transition across freeze, probe, durable CAS, and host
// authority commit instead.
func (s *RuntimeState) ObserveSecurityContextAndCancelInvalidatedJobs(ctx context.Context, current domainsecurity.TurnSecurityContext, timeout time.Duration) error {
	transition, err := s.BeginSecurityContextTransition(ctx, current)
	if err != nil {
		return err
	}
	defer transition.Abort()
	if err := transition.Prepare(ctx, current, timeout); err != nil {
		return err
	}
	return transition.Commit()
}

func (s *RuntimeState) bindingAllowedLocked(binding *domainjob.SecurityBinding) bool {
	if binding == nil || domainjob.ValidateSecurityBinding(binding) != nil {
		return false
	}
	threadKey := securityAuthorityMapKey("thread", binding.TenantID, binding.UserID, binding.ParentThreadID)
	currentThread, threadFound := s.currentByThread[threadKey]
	if !threadFound || !domainjob.SecurityBindingMatchesContext(binding, currentThread) {
		return false
	}
	workspaceKey := securityAuthorityMapKey("workspace", binding.TenantID, binding.UserID, binding.ParentWorkspaceRealPath)
	currentWorkspace, workspaceFound := s.currentByWorkspace[workspaceKey]
	if !workspaceFound || !domainjob.SecurityBindingMatchesWorkspaceScope(binding, currentWorkspace) {
		return false
	}
	return true
}

func (s *RuntimeState) bindingTransitioningLocked(binding *domainjob.SecurityBinding) bool {
	if binding == nil {
		return false
	}
	threadKey := securityAuthorityMapKey("thread", binding.TenantID, binding.UserID, binding.ParentThreadID)
	workspaceKey := securityAuthorityMapKey("workspace", binding.TenantID, binding.UserID, binding.ParentWorkspaceRealPath)
	return s.transitionThreads[threadKey] || s.transitionWorkspaces[workspaceKey]
}

func (s *RuntimeState) reservePendingChildSteer(ctx context.Context, record domainjob.Record) (func(), error) {
	if s == nil || !record.Background || domainjob.ValidateSecurityBinding(record.SecurityBinding) != nil {
		return nil, errors.New("pending child steer registration is unavailable")
	}
	s.mu.Lock()
	control := s.backgroundJobs[record.ID]
	s.mu.Unlock()
	if control == nil {
		return nil, errors.New("pending child steer registration is unavailable")
	}
	// Never wait for the start boundary while holding RuntimeState.mu.
	release, err := control.startBarrier.reservePendingSteer(ctx)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	valid := s.backgroundJobs[record.ID] == control && !s.backgroundClosing &&
		control.binding != nil && control.binding.BindingDigest == record.SecurityBinding.BindingDigest &&
		s.bindingAllowedLocked(record.SecurityBinding) && !s.bindingTransitioningLocked(record.SecurityBinding)
	s.mu.Unlock()
	if !valid {
		release()
		return nil, errors.New("pending child steer registration changed")
	}
	return release, nil
}

func securityAuthorityMapKey(kind string, values ...string) string {
	var key strings.Builder
	key.WriteString(kind)
	for _, value := range values {
		key.WriteByte(':')
		key.WriteString(strconv.Itoa(len(value)))
		key.WriteByte(':')
		key.WriteString(value)
	}
	return key.String()
}

func contextDone(ctx context.Context) <-chan struct{} {
	if ctx == nil {
		return nil
	}
	return ctx.Done()
}

func contextError(ctx context.Context) error {
	if ctx == nil || ctx.Err() == nil {
		return errors.New("background job cancellation was interrupted")
	}
	return ctx.Err()
}

func (s *RuntimeState) UnregisterBackgroundJob(id string) {
	if s == nil {
		return
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return
	}
	s.mu.Lock()
	control := s.backgroundJobs[id]
	s.mu.Unlock()
	if control != nil {
		control.startBarrier.CancelAndWait()
		control.closeDone()
		s.mu.Lock()
		if s.backgroundJobs[id] == control {
			delete(s.backgroundJobs, id)
		}
		s.mu.Unlock()
	}
}

// WaitBackgroundJobSettled waits for the worker's final unregister
// acknowledgement, which occurs only after parent completion/delivery writes.
// An absent control is already settled; it must not be confused with a
// terminal durable job whose worker is still performing tail writes.
func (s *RuntimeState) WaitBackgroundJobSettled(id string, timeout time.Duration) (bool, error) {
	if s == nil || strings.TrimSpace(id) == "" {
		return true, nil
	}
	control, ok := s.backgroundJobControl(strings.TrimSpace(id))
	if !ok {
		return true, nil
	}
	return waitForBackgroundJobControl(control, timeout)
}

func (s *RuntimeState) CancelBackgroundJob(id string) bool {
	control, ok := s.backgroundJobControl(id)
	if !ok {
		return false
	}
	control.requestCancel()
	return true
}

func (s *RuntimeState) CancelBackgroundJobAndWait(id string, timeout time.Duration) (found bool, stopped bool, err error) {
	control, ok := s.backgroundJobControl(id)
	if !ok {
		return false, false, nil
	}
	control.requestCancel()
	stopped, err = waitForBackgroundJobControl(control, timeout)
	return true, stopped, err
}

// CancelBoundBackgroundJobsForParentThreadAndWait closes every exact
// principal-owned child mutator for one parent thread and waits for its final
// unregister acknowledgement. Turn start invokes this before reading its
// post-quiescence durable baseline so an acknowledged completion tail cannot
// race the later append CAS.
func (s *RuntimeState) CancelBoundBackgroundJobsForParentThreadAndWait(
	ctx context.Context,
	threadID, tenantID, userID string,
	timeout time.Duration,
) error {
	threadID = strings.TrimSpace(threadID)
	tenantID = strings.TrimSpace(tenantID)
	userID = strings.TrimSpace(userID)
	if s == nil || ctx == nil || threadID == "" || tenantID == "" || userID == "" || timeout <= 0 {
		return errors.New("parent thread background cancellation scope is invalid")
	}
	s.mu.Lock()
	controls := make([]*backgroundJobControl, 0)
	for _, control := range s.backgroundJobs {
		if control == nil || control.cancel == nil || domainjob.ValidateSecurityBinding(control.binding) != nil {
			continue
		}
		binding := control.binding
		if binding.ParentThreadID == threadID && binding.TenantID == tenantID && binding.UserID == userID {
			controls = append(controls, control)
		}
	}
	s.mu.Unlock()
	for _, control := range controls {
		control.requestCancel()
	}
	return waitForInvalidatedControls(ctx, controls, timeout)
}

func (s *RuntimeState) CancelBackgroundJobsAndWait(timeout time.Duration) (found int, stopped int, err error) {
	controls := s.closeBackgroundAdmissionAndControls()
	found = len(controls)
	for _, control := range controls {
		control.requestCancel()
	}
	if found == 0 {
		return 0, 0, nil
	}
	if timeout <= 0 {
		return found, 0, fmt.Errorf("%w: stopped 0 of %d", ErrBackgroundJobStopTimeout, found)
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	for index, control := range controls {
		select {
		case <-control.done:
			stopped++
		case <-timer.C:
			for _, remaining := range controls[index:] {
				select {
				case <-remaining.done:
					stopped++
				default:
				}
			}
			return found, stopped, fmt.Errorf("%w: stopped %d of %d", ErrBackgroundJobStopTimeout, stopped, found)
		}
	}
	return found, stopped, nil
}

func (s *RuntimeState) closeBackgroundAdmissionAndControls() []*backgroundJobControl {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.backgroundClosing {
		s.backgroundClosing = true
		if s.admissionClosed == nil {
			s.admissionClosed = make(chan struct{})
		}
		close(s.admissionClosed)
	}
	controls := make([]*backgroundJobControl, 0, len(s.backgroundJobs))
	for _, control := range s.backgroundJobs {
		if control != nil && control.cancel != nil {
			controls = append(controls, control)
		}
	}
	return controls
}

func (s *RuntimeState) backgroundJobControl(id string) (*backgroundJobControl, bool) {
	if s == nil {
		return nil, false
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, false
	}
	s.mu.Lock()
	control := s.backgroundJobs[id]
	s.mu.Unlock()
	if control == nil || control.cancel == nil {
		return nil, false
	}
	return control, true
}

func (s *RuntimeState) BackgroundJobCancels() []context.CancelFunc {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	cancels := make([]context.CancelFunc, 0, len(s.backgroundJobs))
	for _, control := range s.backgroundJobs {
		if control != nil && control.cancel != nil {
			cancels = append(cancels, control.requestCancel)
		}
	}
	return cancels
}

func (s *RuntimeState) ActiveBackgroundJobIDs() []string {
	if s == nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	ids := make([]string, 0, len(s.backgroundJobs))
	for id, control := range s.backgroundJobs {
		if control != nil && control.cancel != nil {
			ids = append(ids, id)
		}
	}
	return ids
}

type BackgroundJobPauseSnapshot struct {
	JobID          string
	PauseRequestID string
	Status         string
	RequestedAt    time.Time
	PausedAt       time.Time
}

func (s *RuntimeState) RequestBackgroundJobPause(id string, pauseRequestID string, now time.Time) (BackgroundJobPauseSnapshot, bool) {
	control, ok := s.backgroundJobControl(id)
	if !ok {
		return BackgroundJobPauseSnapshot{}, false
	}
	if now.IsZero() {
		now = time.Now().UTC()
	}
	pauseRequestID = strings.TrimSpace(pauseRequestID)
	control.mu.Lock()
	defer control.mu.Unlock()
	if control.resumeCh == nil {
		control.resumeCh = make(chan struct{})
	}
	control.pauseRequested = true
	control.pauseRequestID = pauseRequestID
	control.requestedAt = now
	status := "pause_requested"
	if control.paused {
		status = "paused"
	}
	return BackgroundJobPauseSnapshot{
		JobID:          strings.TrimSpace(id),
		PauseRequestID: control.pauseRequestID,
		Status:         status,
		RequestedAt:    control.requestedAt,
		PausedAt:       control.pausedAt,
	}, true
}

func (s *RuntimeState) ClearBackgroundJobPauseRequest(id string, pauseRequestID string) bool {
	control, ok := s.backgroundJobControl(id)
	if !ok {
		return false
	}
	control.mu.Lock()
	defer control.mu.Unlock()
	if strings.TrimSpace(pauseRequestID) != "" && strings.TrimSpace(control.pauseRequestID) != strings.TrimSpace(pauseRequestID) {
		return false
	}
	control.pauseRequested = false
	if !control.paused {
		control.pauseRequestID = ""
		control.requestedAt = time.Time{}
		control.resumeCh = nil
	}
	return true
}

func (s *RuntimeState) ResumeBackgroundJob(id string, pauseRequestID string) (BackgroundJobPauseSnapshot, bool) {
	control, ok := s.backgroundJobControl(id)
	if !ok {
		return BackgroundJobPauseSnapshot{}, false
	}
	control.mu.Lock()
	if strings.TrimSpace(pauseRequestID) != "" && strings.TrimSpace(control.pauseRequestID) != strings.TrimSpace(pauseRequestID) {
		control.mu.Unlock()
		return BackgroundJobPauseSnapshot{}, false
	}
	if !control.paused {
		control.mu.Unlock()
		return BackgroundJobPauseSnapshot{}, false
	}
	snapshot := BackgroundJobPauseSnapshot{
		JobID:          strings.TrimSpace(id),
		PauseRequestID: control.pauseRequestID,
		Status:         "resume_requested",
		RequestedAt:    control.requestedAt,
		PausedAt:       control.pausedAt,
	}
	resumeCh := control.resumeCh
	control.resumeCh = nil
	control.paused = false
	control.mu.Unlock()
	if resumeCh != nil {
		close(resumeCh)
	}
	return snapshot, true
}

func (s *RuntimeState) BackgroundJobPauseSnapshot(id string) (BackgroundJobPauseSnapshot, bool) {
	control, ok := s.backgroundJobControl(id)
	if !ok {
		return BackgroundJobPauseSnapshot{}, false
	}
	control.mu.Lock()
	defer control.mu.Unlock()
	status := ""
	if control.paused {
		status = "paused"
	} else if control.pauseRequested {
		status = "pause_requested"
	}
	if status == "" {
		return BackgroundJobPauseSnapshot{}, false
	}
	return BackgroundJobPauseSnapshot{
		JobID:          strings.TrimSpace(id),
		PauseRequestID: control.pauseRequestID,
		Status:         status,
		RequestedAt:    control.requestedAt,
		PausedAt:       control.pausedAt,
	}, true
}

type BackgroundJobPauseCallbacks struct {
	OnPaused  func(BackgroundJobPauseSnapshot) error
	OnResumed func(BackgroundJobPauseSnapshot) error
}

func (s *RuntimeState) WaitIfBackgroundJobPauseRequested(ctx context.Context, id string, callbacks BackgroundJobPauseCallbacks) (bool, error) {
	control, ok := s.backgroundJobControl(id)
	if !ok {
		return false, nil
	}
	control.mu.Lock()
	if !control.pauseRequested && !control.paused {
		control.mu.Unlock()
		return false, nil
	}
	if control.resumeCh == nil {
		control.resumeCh = make(chan struct{})
	}
	control.pauseRequested = false
	control.paused = true
	control.pausedAt = time.Now().UTC()
	snapshot := BackgroundJobPauseSnapshot{
		JobID:          strings.TrimSpace(id),
		PauseRequestID: control.pauseRequestID,
		Status:         "paused",
		RequestedAt:    control.requestedAt,
		PausedAt:       control.pausedAt,
	}
	resumeCh := control.resumeCh
	control.mu.Unlock()
	if callbacks.OnPaused != nil {
		if err := callbacks.OnPaused(snapshot); err != nil {
			return true, err
		}
	}
	select {
	case <-ctx.Done():
		return true, ctx.Err()
	case <-resumeCh:
	}
	if err := ctx.Err(); err != nil {
		return true, err
	}
	control.mu.Lock()
	control.paused = false
	control.pauseRequested = false
	snapshot = BackgroundJobPauseSnapshot{
		JobID:          strings.TrimSpace(id),
		PauseRequestID: control.pauseRequestID,
		Status:         "resuming",
		RequestedAt:    control.requestedAt,
		PausedAt:       control.pausedAt,
	}
	control.pauseRequestID = ""
	control.requestedAt = time.Time{}
	control.pausedAt = time.Time{}
	control.resumeCh = nil
	control.mu.Unlock()
	if callbacks.OnResumed != nil {
		if err := callbacks.OnResumed(snapshot); err != nil {
			return true, err
		}
	}
	return true, nil
}

func (c *backgroundJobControl) closeDone() {
	if c == nil {
		return
	}
	c.once.Do(func() {
		close(c.done)
	})
}

func (c *backgroundJobControl) resume() {
	if c == nil {
		return
	}
	c.mu.Lock()
	if c.resumeCh == nil {
		c.mu.Unlock()
		return
	}
	resumeCh := c.resumeCh
	c.resumeCh = nil
	c.paused = false
	c.pauseRequested = false
	c.mu.Unlock()
	close(resumeCh)
}

func (c *backgroundJobControl) requestCancel() {
	if c == nil {
		return
	}
	// Do not hold RuntimeState.mu or the context-effect gate here. Closing the
	// atomic start authority never waits for StartTracked; the worker's later
	// Unregister is the only path that waits before acknowledging done.
	c.startBarrier.Cancel()
	c.cancel()
	c.resume()
}

func waitForBackgroundJobControl(control *backgroundJobControl, timeout time.Duration) (bool, error) {
	if control == nil {
		return false, errors.New("background job control is unavailable")
	}
	if timeout <= 0 {
		return false, ErrBackgroundJobStopTimeout
	}
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-control.done:
		return true, nil
	case <-timer.C:
		select {
		case <-control.done:
			return true, nil
		default:
		}
		return false, ErrBackgroundJobStopTimeout
	}
}

func (s *RuntimeState) OutputCursor(threadID string, jobID string) int {
	if s == nil {
		return 0
	}
	key := OutputCursorKey(threadID, jobID)
	if key == "" {
		return 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.outputCursors[key]
}

func (s *RuntimeState) SetOutputCursor(threadID string, jobID string, cursor int) {
	if s == nil {
		return
	}
	key := OutputCursorKey(threadID, jobID)
	if key == "" {
		return
	}
	if cursor < 0 {
		cursor = 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.outputCursors == nil {
		s.outputCursors = map[string]int{}
	}
	s.outputCursors[key] = cursor
}

func OutputCursorKey(threadID string, jobID string) string {
	threadID = strings.TrimSpace(threadID)
	jobID = strings.TrimSpace(jobID)
	if threadID == "" || jobID == "" {
		return ""
	}
	return threadID + "\x00" + jobID
}

func (s *RuntimeState) AcquireSlot(ctx context.Context, limit int) (func(), error) {
	if s == nil {
		return func() {}, nil
	}
	if limit < 1 {
		limit = 1
	}
	waiter := make(chan struct{})
	s.mu.Lock()
	if s.active < limit && len(s.waiters) == 0 {
		s.active++
		s.mu.Unlock()
		return s.ReleaseSlot, nil
	}
	s.waiters = append(s.waiters, waiter)
	s.mu.Unlock()
	select {
	case <-waiter:
		return s.ReleaseSlot, nil
	case <-ctx.Done():
		removed := false
		s.mu.Lock()
		for index, candidate := range s.waiters {
			if candidate == waiter {
				s.waiters = append(s.waiters[:index], s.waiters[index+1:]...)
				removed = true
				break
			}
		}
		s.mu.Unlock()
		if !removed {
			return s.ReleaseSlot, nil
		}
		return nil, ctx.Err()
	}
}

func (s *RuntimeState) ReleaseSlot() {
	if s == nil {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.waiters) > 0 {
		next := s.waiters[0]
		s.waiters = s.waiters[1:]
		close(next)
		return
	}
	if s.active > 0 {
		s.active--
	}
}

func (s *RuntimeState) ActiveQueued() (int, int) {
	if s == nil {
		return 0, 0
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.active, len(s.waiters)
}
