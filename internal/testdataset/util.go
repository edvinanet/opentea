// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package testdataset

import "strconv"

// itoa is strconv.Itoa, aliased for brevity at the many call sites below
// that build deterministic names/identifiers/UUID seeds from a loop index.
func itoa(i int) string { return strconv.Itoa(i) }

// ordinalName builds "<base> NN" with a zero-padded two-digit index --
// used for component/component-release names across the larger datasets,
// where a plain "<base> 7" would sort/group inconsistently against
// "<base> 10".
func ordinalName(base string, i int) string {
	return base + " " + pad2(i)
}

func pad2(i int) string {
	if i < 10 {
		return "0" + itoa(i)
	}
	return itoa(i)
}
