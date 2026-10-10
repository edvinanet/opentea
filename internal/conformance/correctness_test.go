// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package conformance

import "testing"

func TestCorrectnessTrackerTallies(t *testing.T) {
	c := NewCorrectnessTracker()
	c.Check(true, "a")
	c.Check(false, "b")
	c.Check(true, "c")

	report := c.Report()
	if report.Total != 3 {
		t.Fatalf("Total = %d, want 3", report.Total)
	}
	if report.Passed != 2 {
		t.Fatalf("Passed = %d, want 2", report.Passed)
	}
	if len(report.Failed) != 1 || report.Failed[0] != "b" {
		t.Fatalf("Failed = %v, want [\"b\"]", report.Failed)
	}
	if got, want := report.Percent(), 100*2.0/3.0; got != want {
		t.Fatalf("Percent() = %v, want %v", got, want)
	}
}

func TestCorrectnessReportPercentZeroTotal(t *testing.T) {
	if (CorrectnessReport{}).Percent() != 0 {
		t.Fatal("Percent() on a zero-Total report should be 0, not NaN/panic")
	}
}
