// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package trust

import "testing"

func TestCanonicalizeKeySorting(t *testing.T) {
	a := map[string]any{"b": 1, "a": 2, "c": 3}
	b := map[string]any{"c": 3, "a": 2, "b": 1}

	outA, err := Canonicalize(a)
	if err != nil {
		t.Fatalf("Canonicalize: %v", err)
	}
	outB, err := Canonicalize(b)
	if err != nil {
		t.Fatalf("Canonicalize: %v", err)
	}
	if string(outA) != string(outB) {
		t.Fatalf("canonical output differs by input key order: %s != %s", outA, outB)
	}
	if string(outA) != `{"a":2,"b":1,"c":3}` {
		t.Fatalf("unexpected canonical output: %s", outA)
	}
}

func TestCanonicalizeKnownVectors(t *testing.T) {
	tests := []struct {
		name string
		in   any
		want string
	}{
		{"empty object", map[string]any{}, `{}`},
		{"nested object", map[string]any{"b": map[string]any{"y": 2, "x": 1}, "a": "hello"}, `{"a":"hello","b":{"x":1,"y":2}}`},
		{"array preserves order", map[string]any{"list": []any{3, 1, 2}}, `{"list":[3,1,2]}`},
		{"string escaping stays minimal", map[string]any{"note": "a < b & c > d"}, `{"note":"a < b & c > d"}`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Canonicalize(tt.in)
			if err != nil {
				t.Fatalf("Canonicalize: %v", err)
			}
			if string(got) != tt.want {
				t.Fatalf("Canonicalize(%v) = %s, want %s", tt.in, got, tt.want)
			}
		})
	}
}
