package main

import (
	"fmt"
	"regexp"
	"strings"
)

var placeholderRe = regexp.MustCompile(`\{\{([a-zA-Z0-9_]+)((?:\.[a-zA-Z0-9_]+)+)\}\}`)

// substituteString resolves every {{name.field.subfield}} placeholder in s
// against previously-saved response data.
func substituteString(s string, vars map[string]any) (string, error) {
	var firstErr error
	result := placeholderRe.ReplaceAllStringFunc(s, func(match string) string {
		groups := placeholderRe.FindStringSubmatch(match)
		name := groups[1]
		fieldPath := strings.TrimPrefix(groups[2], ".")

		val, ok := vars[name]
		if !ok {
			if firstErr == nil {
				firstErr = fmt.Errorf("unknown variable %q referenced in %q (was it saved by an earlier step?)", name, s)
			}
			return match
		}
		resolved, err := lookupField(val, fieldPath)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			return match
		}
		return fmt.Sprintf("%v", resolved)
	})
	if firstErr != nil {
		return "", firstErr
	}
	return result, nil
}

func lookupField(v any, dotted string) (any, error) {
	cur := v
	for _, part := range strings.Split(dotted, ".") {
		m, ok := cur.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("cannot look up field %q: not an object", part)
		}
		cur, ok = m[part]
		if !ok {
			return nil, fmt.Errorf("field %q not found", part)
		}
	}
	return cur, nil
}

// substituteJSON recursively applies substituteString to every string leaf
// in an arbitrary decoded-JSON value (map[string]any / []any / scalars).
func substituteJSON(v any, vars map[string]any) (any, error) {
	switch val := v.(type) {
	case string:
		return substituteString(val, vars)
	case map[string]any:
		out := make(map[string]any, len(val))
		for k, vv := range val {
			sub, err := substituteJSON(vv, vars)
			if err != nil {
				return nil, err
			}
			out[k] = sub
		}
		return out, nil
	case []any:
		out := make([]any, len(val))
		for i, vv := range val {
			sub, err := substituteJSON(vv, vars)
			if err != nil {
				return nil, err
			}
			out[i] = sub
		}
		return out, nil
	default:
		return v, nil
	}
}
