package handlers

// ValidationResult describes the outcome of verifying a request payload or parameters.
type ValidationResult struct {
	IsValid bool
	Error   string
}
