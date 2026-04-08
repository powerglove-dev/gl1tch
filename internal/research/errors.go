package research

import "errors"

// errorsJoin is a thin wrapper around errors.Join so the rest of the package
// reads errorsJoin(...) instead of importing "errors" everywhere just for
// one call. Centralising it here also lets future versions of Go's errors
// package be swapped in cleanly.
func errorsJoin(errs []error) error {
	return errors.Join(errs...)
}
