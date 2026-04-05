package netinterface

import (
	"testing"
	"time"
)

func TestUpdateDailyTotals_SameDay(t *testing.T) {
	n := &NetInterface{}
	n.data.CurrentDayIndex = 0

	// 10 seconds elapsed, 100 bytes in, 200 bytes out
	now := time.Date(2023, 10, 10, 12, 0, 10, 0, time.UTC)
	n.updateDailyTotals(now, 10, 100, 200)

	if n.data.DailyBytesIn[0] != 100 {
		t.Errorf("Expected 100 DailyBytesIn, got %d", n.data.DailyBytesIn[0])
	}
	if n.data.DailyBytesOut[0] != 200 {
		t.Errorf("Expected 200 DailyBytesOut, got %d", n.data.DailyBytesOut[0])
	}
	if n.data.CurrentDayIndex != 0 {
		t.Errorf("Expected CurrentDayIndex 0, got %d", n.data.CurrentDayIndex)
	}
}

func TestUpdateDailyTotals_CrossMidnight(t *testing.T) {
	n := &NetInterface{}
	n.data.CurrentDayIndex = 0

	// Interval starts 5 seconds before midnight, ends 5 seconds after midnight. Total 10 seconds.
	// 100 bytes in, 200 out total -> 10 bytes/sec in, 20 bytes/sec out
	// 5 seconds in day 0 (50 in, 100 out)
	// 5 seconds in day 1 (50 in, 100 out)
	now := time.Date(2023, 10, 11, 0, 0, 5, 0, time.UTC)
	n.updateDailyTotals(now, 10, 100, 200)

	if n.data.DailyBytesIn[0] != 50 {
		t.Errorf("Expected Day 0 DailyBytesIn 50, got %d", n.data.DailyBytesIn[0])
	}
	if n.data.DailyBytesOut[0] != 100 {
		t.Errorf("Expected Day 0 DailyBytesOut 100, got %d", n.data.DailyBytesOut[0])
	}

	if n.data.CurrentDayIndex != 1 {
		t.Errorf("Expected CurrentDayIndex 1, got %d", n.data.CurrentDayIndex)
	}
	if n.data.DailyBytesIn[1] != 50 {
		t.Errorf("Expected Day 1 DailyBytesIn 50, got %d", n.data.DailyBytesIn[1])
	}
	if n.data.DailyBytesOut[1] != 100 {
		t.Errorf("Expected Day 1 DailyBytesOut 100, got %d", n.data.DailyBytesOut[1])
	}
}

func TestUpdateDailyTotals_CrossMultipleMidnights(t *testing.T) {
	n := &NetInterface{}
	n.data.CurrentDayIndex = 0

	// interval covers exactly 48 hours (172800 seconds)
	// starts at midnight, spans exactly two full days.
	// 172800 bytes in, 345600 out -> 1 byte/sec in, 2 bytes/sec out
	// Day 0: 86400 in, 172800 out
	// Day 1: 86400 in, 172800 out
	now := time.Date(2023, 10, 12, 0, 0, 0, 0, time.UTC)
	n.updateDailyTotals(now, 172800, 172800, 345600)

	if n.data.CurrentDayIndex != 2 {
		t.Errorf("Expected CurrentDayIndex 2, got %d", n.data.CurrentDayIndex)
	}

	if n.data.DailyBytesIn[0] != 86400 {
		t.Errorf("Expected Day 0 DailyBytesIn 86400, got %d", n.data.DailyBytesIn[0])
	}
	if n.data.DailyBytesIn[1] != 86400 {
		t.Errorf("Expected Day 1 DailyBytesIn 86400, got %d", n.data.DailyBytesIn[1])
	}
}

func TestUpdateDailyTotals_RolloverHistoryIndex(t *testing.T) {
	n := &NetInterface{}
	// Set index to the end of the history buffer
	n.data.CurrentDayIndex = 63 // NetInterfaceDailyHistorySize - 1
	n.data.DailyBytesIn[63] = 10

	// interval crosses midnight, jumping to the next day
	now := time.Date(2023, 10, 11, 0, 0, 5, 0, time.UTC)
	n.updateDailyTotals(now, 10, 100, 200)

	// Day 63 should get the first 5 seconds
	if n.data.DailyBytesIn[63] != 60 { // 10 original + 50 new
		t.Errorf("Expected Day 63 DailyBytesIn 60, got %d", n.data.DailyBytesIn[63])
	}

	// The next day should rollover to index 0
	if n.data.CurrentDayIndex != 0 {
		t.Errorf("Expected CurrentDayIndex to rollover to 0, got %d", n.data.CurrentDayIndex)
	}
	if n.data.DailyBytesIn[0] != 50 {
		t.Errorf("Expected Day 0 DailyBytesIn 50, got %d", n.data.DailyBytesIn[0])
	}
}
