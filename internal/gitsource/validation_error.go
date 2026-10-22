// The validation error this package reports for invalid input.

package gitsource

import "github.com/fabricahq/trailguide/internal/errs"

// ValidationError reports an invalid repository address or ref at a location, such as a configuration key.
type ValidationError struct {
	// Location is a field path or flag. It is diagnostic text, never a path to access.
	Location string
	// Problem explains the failure to a human. Callers must not branch on its text.
	Problem string
}

var _ errs.ValidationError = (*ValidationError)(nil)

// Error returns the location and the problem.
func (e *ValidationError) Error() string { return e.Location + ": " + e.Problem }

// ValidationLocation returns Location.
func (e *ValidationError) ValidationLocation() string { return e.Location }

// ValidationProblem returns Problem.
func (e *ValidationError) ValidationProblem() string { return e.Problem }

// invalid returns a *ValidationError reporting problem at location.
func invalid(location, problem string) error {
	return &ValidationError{Location: location, Problem: problem}
}
