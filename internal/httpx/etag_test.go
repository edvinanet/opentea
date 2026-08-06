package httpx

import (
	"net/http/httptest"
	"testing"
)

func TestBuildETag(t *testing.T) {
	got := BuildETag("artifact", "550e8400-e29b-41d4-a716-446655440000", "7", "3")
	want := `"artifact:550e8400-e29b-41d4-a716-446655440000:7:3"`
	if got != want {
		t.Fatalf("BuildETag = %q, want %q", got, want)
	}
}

func TestMatchesIfNoneMatch(t *testing.T) {
	const current = `"product:abc:1"`
	cases := []struct {
		name    string
		headers []string
		want    bool
	}{
		{"empty", nil, false},
		{"exact match", []string{`"product:abc:1"`}, true},
		{"non-matching", []string{`"product:abc:2"`}, false},
		{"wildcard", []string{"*"}, true},
		{"multiple comma-separated, second matches", []string{`"other:1", "product:abc:1"`}, true},
		{"multiple comma-separated, none match", []string{`"other:1", "another:2"`}, false},
		{"multiple header lines, second matches", []string{`"other:1"`, `"product:abc:1"`}, true},
		{"weak client validator matches strong server one", []string{`W/"product:abc:1"`}, true},
		{"extra whitespace around values", []string{` "other:1" , "product:abc:1" `}, true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := MatchesIfNoneMatch(c.headers, current); got != c.want {
				t.Errorf("MatchesIfNoneMatch(%v, %q) = %v, want %v", c.headers, current, got, c.want)
			}
		})
	}
}

func TestWriteConditionalMatch(t *testing.T) {
	const etag = `"product:abc:1"`
	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("If-None-Match", etag)
	w := httptest.NewRecorder()

	notModified := WriteConditional(w, r, etag, "public, max-age=60")
	if !notModified {
		t.Fatal("WriteConditional = false, want true (matching If-None-Match)")
	}
	if w.Code != 304 {
		t.Fatalf("status = %d, want 304", w.Code)
	}
	if w.Body.Len() != 0 {
		t.Fatalf("body = %q, want empty", w.Body.String())
	}
	if got := w.Header().Get("ETag"); got != etag {
		t.Errorf("ETag header = %q, want %q", got, etag)
	}
	if got := w.Header().Get("Cache-Control"); got != "public, max-age=60" {
		t.Errorf("Cache-Control header = %q", got)
	}
}

func TestWriteConditionalNoMatch(t *testing.T) {
	const etag = `"product:abc:2"`
	r := httptest.NewRequest("GET", "/", nil)
	r.Header.Set("If-None-Match", `"product:abc:1"`)
	w := httptest.NewRecorder()

	notModified := WriteConditional(w, r, etag, "public, max-age=60")
	if notModified {
		t.Fatal("WriteConditional = true, want false (stale If-None-Match)")
	}
	if got := w.Header().Get("ETag"); got != etag {
		t.Errorf("ETag header = %q, want %q (still set so the caller's normal response carries it)", got, etag)
	}
	// The caller is responsible for writing the actual 200 response body
	// (WriteConditional itself must not call WriteHeader here).
	if w.Code != 200 {
		t.Fatalf("status = %d, want default 200 (no WriteHeader call yet)", w.Code)
	}
}

func TestWriteConditionalNoHeader(t *testing.T) {
	const etag = `"product:abc:1"`
	r := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()

	if WriteConditional(w, r, etag, "public, max-age=60") {
		t.Fatal("WriteConditional = true, want false (no If-None-Match header sent at all)")
	}
}
