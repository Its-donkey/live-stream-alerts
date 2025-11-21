package service

import "errors"

var (
	// ErrValidation indicates the supplied payload is invalid.
	ErrValidation = errors.New("validation error")
	// ErrUpstream signals an error occurred while contacting an upstream service.
	ErrUpstream = errors.New("upstream error")
)
