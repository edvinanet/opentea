// SPDX-License-Identifier: BSD-2-Clause
// SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

package openteapublisher

import "errors"

// ErrNotFound is returned by any Get/Delete method when the identified row
// doesn't exist.
var ErrNotFound = errors.New("openteapublisher: not found")
