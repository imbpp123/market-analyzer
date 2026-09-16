package domain

import "fmt"

type ValidationError struct {
	Field string
	Rule  string
}

func (e *ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Field, e.Rule)
}

func invalid(field, rule string) *ValidationError {
	return &ValidationError{Field: field, Rule: rule}
}
