package alerting

import (
	"testing"
	"time"

	"github.com/avanha/pmaas-plugin-netmon/data"
	"github.com/avanha/pmaas-spi/alert"
)

var start = time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)

func at(minutes int) time.Time { return start.Add(time.Duration(minutes) * time.Minute) }

func TestTracker_RaisesOnceAfterTheTimeAndClearsWhenTheHostIsBack(t *testing.T) {
	policy := Policy{After: 2 * time.Minute, Severity: alert.Always(alert.SeverityCritical)}
	var tracker Tracker

	for _, c := range []struct {
		minute       int
		reachability int
		want         Action
		down         time.Duration
	}{
		{0, data.ReachabilityReachable, Nothing, 0},
		{1, data.ReachabilityUnreachable, Nothing, 0},               // down since minute 1
		{2, data.ReachabilityUnreachable, Nothing, time.Minute},     // one minute
		{3, data.ReachabilityUnreachable, Raise, 2 * time.Minute},   // two
		{4, data.ReachabilityUnreachable, Nothing, 3 * time.Minute}, // already raised
		{5, data.ReachabilityUnreachable, Nothing, 4 * time.Minute},
		{6, data.ReachabilityReachable, Clear, 0},
		{7, data.ReachabilityReachable, Nothing, 0},
	} {
		action, severity, down := tracker.Evaluate(policy, c.reachability, at(1), at(c.minute))

		if action != c.want || down != c.down {
			t.Errorf("minute %d: %v after %v, want %v after %v", c.minute, action, down, c.want, c.down)
		}

		if action == Raise && severity != alert.SeverityCritical {
			t.Errorf("minute %d: severity %v", c.minute, severity)
		}
	}
}

func TestTracker_ARaisedAlertIsRaisedAgainWhenTheSeverityChangesAndOnlyThen(t *testing.T) {
	policy := Policy{After: time.Minute, Severity: alert.ByTimeOfDay(alert.SeverityWarning, 7, 0, 23, 0, alert.SeverityCritical)}
	var tracker Tracker

	since := time.Date(2026, 10, 16, 22, 0, 0, 0, time.UTC)
	poll := func(hour, minute int) (Action, alert.Severity) {
		action, severity, _ := tracker.Evaluate(policy, data.ReachabilityUnreachable, since, time.Date(2026, 10, 16, hour, minute, 0, 0, time.UTC))

		return action, severity
	}

	if action, severity := poll(22, 0); action != Nothing || severity != 0 {
		t.Errorf("not down long enough: %v %v", action, severity)
	}

	if action, severity := poll(22, 1); action != Raise || severity != alert.SeverityWarning {
		t.Errorf("first: %v %v", action, severity)
	}

	if action, _ := poll(22, 59); action != Nothing {
		t.Errorf("unchanged: %v", action)
	}

	if action, severity := poll(23, 0); action != Raise || severity != alert.SeverityCritical {
		t.Errorf("escalated: %v %v", action, severity)
	}

	if action, _ := poll(23, 30); action != Nothing {
		t.Errorf("unchanged again: %v", action)
	}

	// The morning: it eases.
	next := time.Date(2026, 10, 17, 7, 0, 0, 0, time.UTC)

	if action, severity, _ := tracker.Evaluate(policy, data.ReachabilityUnreachable, since, next); action != Raise || severity != alert.SeverityWarning {
		t.Errorf("eased: %v %v", action, severity)
	}
}

func TestTracker_APollThatTellsNothingDoesNotClearTheAlert(t *testing.T) {
	policy := Policy{After: time.Minute, Severity: alert.Always(alert.SeverityCritical)}
	var tracker Tracker

	if action, _, _ := tracker.Evaluate(policy, data.ReachabilityUnreachable, at(0), at(5)); action != Raise {
		t.Fatalf("not raised: %v", action)
	}

	if action, _, _ := tracker.Evaluate(policy, data.ReachabilityUnknown, at(0), at(6)); action != Nothing {
		t.Errorf("unknown cleared or raised: %v", action)
	}

	// And it's still raised, so back to reachable clears it.
	if action, _, _ := tracker.Evaluate(policy, data.ReachabilityReachable, at(0), at(7)); action != Clear {
		t.Errorf("reachable: %v", action)
	}
}

func TestTracker_AnAlertCanBeRaisedAgainAfterItWasCleared(t *testing.T) {
	policy := Policy{After: time.Minute, Severity: alert.Always(alert.SeverityCritical)}
	var tracker Tracker

	tracker.Evaluate(policy, data.ReachabilityUnreachable, at(0), at(2))
	tracker.Evaluate(policy, data.ReachabilityReachable, at(0), at(3))

	if action, _, _ := tracker.Evaluate(policy, data.ReachabilityUnreachable, at(10), at(12)); action != Raise {
		t.Errorf("the second outage: %v", action)
	}
}

func TestTracker_ASeverityThatIsNotOneIsAnInfo(t *testing.T) {
	var tracker Tracker

	_, severity, _ := tracker.Evaluate(Policy{After: time.Minute, Severity: alert.Always(alert.Severity(99))},
		data.ReachabilityUnreachable, at(0), at(5))

	if severity != alert.SeverityInfo {
		t.Errorf("got %v", severity)
	}
}

func TestTracker_ExactlyTheTimeIsEnough(t *testing.T) {
	var tracker Tracker
	policy := Policy{After: 2 * time.Minute, Severity: alert.Always(alert.SeverityCritical)}

	if action, _, _ := tracker.Evaluate(policy, data.ReachabilityUnreachable, at(0), at(2).Add(-time.Nanosecond)); action != Nothing {
		t.Error("raised a nanosecond early")
	}

	if action, _, _ := tracker.Evaluate(policy, data.ReachabilityUnreachable, at(0), at(2)); action != Raise {
		t.Error("not raised at exactly the time")
	}
}
