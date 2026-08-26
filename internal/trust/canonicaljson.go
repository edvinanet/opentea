package trust

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// Canonicalize returns v's RFC 8785 JSON Canonicalization Scheme (JCS)
// encoding: object keys sorted, no insignificant whitespace, minimal
// escaping (no HTML-safe over-escaping of "<", ">", "&"). Used for
// evidenceBundleRef digests (SHA-256 of an evidence bundle's canonical
// JSON, tea-trust-architecture 08-evidence-bundle.md Sec 10.3) and object
// digests over JSON objects such as a tea.Collection.
//
// This is deliberately NOT a full JCS implementation: Go's float64
// formatting is not verified against JCS's ECMAScript-number-to-string
// rule, which only matters for non-integer numeric fields. Every shape this
// package canonicalizes today (evidence bundles, collections, artifacts) is
// string/digest/int-heavy with no floating-point fields, so this is
// sufficient. Object-key sort order here is Go's UTF-8 byte-wise string
// comparison (via encoding/json's map-key sorting), which matches JCS's
// UTF-16-code-unit order for every key actually used in this codebase (all
// within the Basic Multilingual Plane) but is not verified to match for
// arbitrary Unicode input. Revisit with a vetted JCS library (e.g.
// github.com/gowebpki/jcs, via the dependency-review skill) before treating
// this as spec-conformant interop with a different trust-architecture
// implementation that might disagree on either point.
func Canonicalize(v any) ([]byte, error) {
	raw, err := marshalNoHTMLEscape(v)
	if err != nil {
		return nil, fmt.Errorf("trust: canonicalize: %w", err)
	}

	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber() // preserve the original numeric literal instead of round-tripping through float64
	var generic any
	if err := dec.Decode(&generic); err != nil {
		return nil, fmt.Errorf("trust: canonicalize: decode: %w", err)
	}

	// encoding/json sorts map[string]any keys when marshaling, which is
	// what gives this its canonical (sorted-key) object ordering -- no
	// custom sort needed here.
	out, err := marshalNoHTMLEscape(generic)
	if err != nil {
		return nil, fmt.Errorf("trust: canonicalize: re-encode: %w", err)
	}
	return out, nil
}

func marshalNoHTMLEscape(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}
