// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package conformance

import "sync"

// CorrectnessTracker counts individual assertions against a server's
// actual responses -- independent of CoverageTracker, which only knows
// whether an operation was called, never whether what it returned was
// right. Every Check call increments total; failures are also recorded by
// label so a report can name exactly what went wrong, not just how much.
type CorrectnessTracker struct {
	mu     sync.Mutex
	total  int
	passed int
	failed []string
}

// NewCorrectnessTracker returns an empty tracker.
func NewCorrectnessTracker() *CorrectnessTracker { return &CorrectnessTracker{} }

// Check records one assertion: ok is whether it held, label names what
// was being checked (used only in the failure report, so make it
// specific -- "Simple product name marker matches" not "check 1").
func (c *CorrectnessTracker) Check(ok bool, label string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.total++
	if ok {
		c.passed++
	} else {
		c.failed = append(c.failed, label)
	}
}

// CorrectnessReport is NewCorrectnessTracker's accumulated result.
type CorrectnessReport struct {
	Passed int
	Total  int
	Failed []string // labels of every failed Check call, in the order they occurred
}

// Percent is Passed/Total as a percentage, or 0 if Total is 0.
func (r CorrectnessReport) Percent() float64 {
	if r.Total == 0 {
		return 0
	}
	return 100 * float64(r.Passed) / float64(r.Total)
}

// Report summarizes every Check call made so far.
func (c *CorrectnessTracker) Report() CorrectnessReport {
	c.mu.Lock()
	defer c.mu.Unlock()
	failed := make([]string, len(c.failed))
	copy(failed, c.failed)
	return CorrectnessReport{Passed: c.passed, Total: c.total, Failed: failed}
}
