// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package main

import (
	"encoding/json"
	"fmt"
)

// printResult renders v either as pretty-printed JSON (-json) or a plain
// Go-syntax dump -- a first cut; nicer per-type human formatting can be
// layered on later without changing the underlying commands.
func printResult(v any, err error, jsonOut bool) error {
	if err != nil {
		return err
	}
	if jsonOut {
		b, mErr := json.MarshalIndent(v, "", "  ")
		if mErr != nil {
			return mErr
		}
		fmt.Println(string(b))
		return nil
	}
	fmt.Printf("%+v\n", v)
	return nil
}
