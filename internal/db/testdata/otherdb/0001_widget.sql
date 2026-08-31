-- SPDX-License-Identifier: BSD-2-Clause
-- SPDX-FileCopyrightText: 2026 Olle E. Johansson, Edvina AB, Sollentuna, Sweden

-- Fixture for TestOpenWithMigrationsIndependentDatabase -- not a real
-- schema, just proves OpenWithMigrations applies a caller-supplied
-- migrations directory independently of internal/db's own.
CREATE TABLE widget (id INTEGER PRIMARY KEY);
