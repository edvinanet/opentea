package repo

import (
	"errors"
	"time"
)

// ErrImportIdentityConflict is returned by an Import* method (or
// ImportComponentLink) when the caller-supplied identity (uuid, or
// (uuid, version) for versioned entities) already exists locally with
// content that doesn't match what's being imported.
//
// A source server's UUID is only meaningful within that source server --
// two different, unrelated source servers can independently assign the
// same UUID to different content. Import is idempotent (re-importing the
// same bundle, or a second bundle that genuinely describes the same
// entity, is a no-op), but it must not silently treat a same-key,
// different-content collision as "already imported": that would corrupt
// local data by aliasing two unrelated things together with no trace of
// what happened. Callers should surface this as a rejected import, not
// retry or paper over it -- see internal/bundle/import.go, which already
// rolls back the whole import atomically (via Repo.WithTx) on any error,
// this one included.
var ErrImportIdentityConflict = errors.New("repo: imported identity conflicts with existing content")

// setEqual reports whether a and b contain the same elements, ignoring
// order and duplicate count. Appropriate for comparing identifier/checksum/
// etc. lists during import-conflict detection: those tables have no unique
// constraint and no defined ordering or multiplicity semantics at the
// schema level (see internal/db/migrations/0001_init.sql), so two lists
// asserting the "same" content aren't guaranteed to match element-for-
// element even when there's no real conflict.
func setEqual[T comparable](a, b []T) bool {
	toSet := func(s []T) map[T]bool {
		m := make(map[T]bool, len(s))
		for _, v := range s {
			m[v] = true
		}
		return m
	}
	sa, sb := toSet(a), toSet(b)
	if len(sa) != len(sb) {
		return false
	}
	for v := range sa {
		if !sb[v] {
			return false
		}
	}
	return true
}

// timePtrEqual reports whether a and b are both nil, or both non-nil and
// equal per time.Time.Equal (which correctly compares instants regardless
// of monotonic-clock reading or *time.Location representation, unlike ==).
func timePtrEqual(a, b *time.Time) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return a.Equal(*b)
}

// ptrEqual reports whether a and b are both nil, or both non-nil and equal
// by value. Don't use this for *time.Time -- use timePtrEqual instead,
// since == on time.Time doesn't correctly compare instants across
// differing monotonic-clock readings or *time.Location representations.
func ptrEqual[T comparable](a, b *T) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}
