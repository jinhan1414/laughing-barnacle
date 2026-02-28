package agent

import (
	"fmt"
	"time"
)

const (
	defaultA2AInitialInterval      = 2 * time.Second
	defaultA2AMaxInterval          = 20 * time.Second
	defaultA2AMaxTrackingDuration  = 90 * time.Second
	defaultA2AMaxConsecutiveErrors = 3
	defaultA2AMinReconcileInterval = 3 * time.Second
	defaultA2AMaxTrackingRenewals  = 24
)

const (
	asyncTaskTrackerStateIdle       = "idle"
	asyncTaskTrackerStateActive     = "active"
	asyncTaskTrackerStatePaused     = "paused"
	asyncTaskTrackerStateRecovering = "recovering"
)

const (
	asyncTaskTrackerReasonNone                     = ""
	asyncTaskTrackerReasonWindowExhausted          = "tracking_window_exhausted"
	asyncTaskTrackerReasonConsecutiveErrorsReached = "tracking_consecutive_errors_reached"
	asyncTaskTrackerReasonInterrupted              = "tracking_interrupted"
	asyncTaskTrackerReasonReconcileDebounced       = "reconcile_debounced"
	asyncTaskTrackerReasonReconcileError           = "reconcile_error"
	asyncTaskTrackerReasonTrackingRenewed          = "tracking_renewed"
)

type A2ATrackingPolicy struct {
	InitialInterval      time.Duration
	MaxInterval          time.Duration
	MaxTrackingDuration  time.Duration
	MaxConsecutiveErrors int
	MinReconcileInterval time.Duration
	MaxTrackingRenewals  int
}

func defaultA2ATrackingPolicy() A2ATrackingPolicy {
	return A2ATrackingPolicy{
		InitialInterval:      defaultA2AInitialInterval,
		MaxInterval:          defaultA2AMaxInterval,
		MaxTrackingDuration:  defaultA2AMaxTrackingDuration,
		MaxConsecutiveErrors: defaultA2AMaxConsecutiveErrors,
		MinReconcileInterval: defaultA2AMinReconcileInterval,
		MaxTrackingRenewals:  defaultA2AMaxTrackingRenewals,
	}
}

func normalizeA2ATrackingPolicy(input A2ATrackingPolicy) (A2ATrackingPolicy, error) {
	policy := defaultA2ATrackingPolicy()
	if input.InitialInterval > 0 {
		policy.InitialInterval = input.InitialInterval
	}
	if input.MaxInterval > 0 {
		policy.MaxInterval = input.MaxInterval
	}
	if input.MaxTrackingDuration > 0 {
		policy.MaxTrackingDuration = input.MaxTrackingDuration
	}
	if input.MaxConsecutiveErrors > 0 {
		policy.MaxConsecutiveErrors = input.MaxConsecutiveErrors
	}
	if input.MinReconcileInterval > 0 {
		policy.MinReconcileInterval = input.MinReconcileInterval
	}
	if input.MaxTrackingRenewals > 0 {
		policy.MaxTrackingRenewals = input.MaxTrackingRenewals
	}
	if policy.InitialInterval <= 0 {
		return A2ATrackingPolicy{}, fmt.Errorf("a2a tracking policy initial_interval must be > 0")
	}
	if policy.MaxInterval < policy.InitialInterval {
		return A2ATrackingPolicy{}, fmt.Errorf("a2a tracking policy max_interval must be >= initial_interval")
	}
	if policy.MaxTrackingDuration <= 0 {
		return A2ATrackingPolicy{}, fmt.Errorf("a2a tracking policy max_tracking_duration must be > 0")
	}
	if policy.MaxConsecutiveErrors <= 0 {
		return A2ATrackingPolicy{}, fmt.Errorf("a2a tracking policy max_consecutive_errors must be > 0")
	}
	if policy.MinReconcileInterval < 0 {
		return A2ATrackingPolicy{}, fmt.Errorf("a2a tracking policy min_reconcile_interval must be >= 0")
	}
	if policy.MaxTrackingRenewals <= 0 {
		return A2ATrackingPolicy{}, fmt.Errorf("a2a tracking policy max_tracking_renewals must be > 0")
	}
	return policy, nil
}

func nextBackoffInterval(current time.Duration, policy A2ATrackingPolicy) time.Duration {
	if current <= 0 {
		return policy.InitialInterval
	}
	next := current * 2
	if next > policy.MaxInterval {
		return policy.MaxInterval
	}
	return next
}
