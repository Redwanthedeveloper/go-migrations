package go_migrations

import "errors"

// ErrNoChanges is returned when models match the current database schema.
var ErrNoChanges = errors.New("no changes detected")

