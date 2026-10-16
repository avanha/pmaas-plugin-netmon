// Package alerting decides when a host that is unreachable is worth an alert, and how severe. It knows nothing of the server or
// of how the host is polled: it is told what each poll found.
package alerting

import (
	"time"

	"github.com/avanha/pmaas-plugin-netmon/data"
	"github.com/avanha/pmaas-spi/alert"
)

// Policy is when a host is worth an alert.
type Policy struct {
	// After is how long the host has to have been unreachable.
	After time.Duration

	// Severity says how severe the alert is at a time.
	Severity alert.SeverityFunc
}

// Action is what to do about the alert after a poll.
type Action int

const (
	// Nothing is that the alert is as it should be.
	Nothing Action = iota

	// Raise is to raise the alert, or raise it again at a new severity.
	Raise

	// Clear is to clear the alert.
	Clear
)

// Tracker remembers what has been raised for one host, so that only a change is acted on.
type Tracker struct {
	raised   bool
	severity alert.Severity
}

// Evaluate says what to do after a poll at the given time that found the host in the given state. reachability is one of
// the data package's Reachability constants, and unreachableSince is when it last became unreachable.
//
// While the host has been unreachable for the policy's time it is raised, at the severity for the time, and raised again if
// that changes. When it is reachable again the alert is cleared. When it is not known whether it is reachable, or it has not
// been unreachable long enough, nothing is done: an alert once raised is not cleared by a poll that tells nothing.
func (t *Tracker) Evaluate(policy Policy, reachability int, unreachableSince time.Time, now time.Time) (Action, alert.Severity, time.Duration) {
	switch reachability {
	case data.ReachabilityReachable:
		if t.raised {
			t.raised = false

			return Clear, 0, 0
		}

		return Nothing, 0, 0
	case data.ReachabilityUnreachable:
		down := now.Sub(unreachableSince)
		if down < policy.After {
			return Nothing, 0, down
		}

		severity := policy.Severity(now).Normalize()
		if t.raised && t.severity == severity {
			return Nothing, severity, down
		}

		t.raised, t.severity = true, severity

		return Raise, severity, down
	default:
		return Nothing, 0, 0
	}
}
