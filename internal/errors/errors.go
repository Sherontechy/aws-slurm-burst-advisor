package errors

import (
	"fmt"
)

// ErrorType represents different categories of errors in the application.
type ErrorType string

const (
	ErrorTypeConfig     ErrorType = "CONFIG"
	ErrorTypeSLURM      ErrorType = "SLURM"
	ErrorTypeAWS        ErrorType = "AWS"
	ErrorTypeValidation ErrorType = "VALIDATION"
	ErrorTypeAnalysis   ErrorType = "ANALYSIS"
	ErrorTypeNetwork    ErrorType = "NETWORK"
	ErrorTypePermission ErrorType = "PERMISSION"
)

// AppError represents a structured application error with context.
type AppError struct {
	Type      ErrorType `json:"type"`
	Message   string    `json:"message"`
	Operation string    `json:"operation"`
	Cause     error     `json:"cause,omitempty"`
	Code      string    `json:"code,omitempty"`
	Retryable bool      `json:"retryable"`
}

// Error implements the error interface.
func (e *AppError) Error() string {
	if e.Operation != "" {
		return fmt.Sprintf("[%s] %s: %s", e.Type, e.Operation, e.Message)
	}
	return fmt.Sprintf("[%s] %s", e.Type, e.Message)
}

// Unwrap returns the underlying cause for error chain compatibility.
func (e *AppError) Unwrap() error {
	return e.Cause
}

// IsRetryable returns true if the error might succeed on retry.
func (e *AppError) IsRetryable() bool {
	return e.Retryable
}

// NewConfigError creates a new configuration-related error.
func NewConfigError(operation, message string, cause error) *AppError {
	return &AppError{
		Type:      ErrorTypeConfig,
		Message:   message,
		Operation: operation,
		Cause:     cause,
		Retryable: false,
	}
}

// NewSLURMError creates a new SLURM-related error.
func NewSLURMError(operation, message string, cause error) *AppError {
	return &AppError{
		Type:      ErrorTypeSLURM,
		Message:   message,
		Operation: operation,
		Cause:     cause,
		Retryable: true, // SLURM commands might be temporarily unavailable
	}
}

// NewAWSError creates a new AWS-related error.
func NewAWSError(operation, message string, cause error) *AppError {
	return &AppError{
		Type:      ErrorTypeAWS,
		Message:   message,
		Operation: operation,
		Cause:     cause,
		Retryable: true, // AWS API calls might be rate limited or temporarily unavailable
	}
}

// NewValidationError creates a new validation error.
func NewValidationError(operation, message string, cause error) *AppError {
	return &AppError{
		Type:      ErrorTypeValidation,
		Message:   message,
		Operation: operation,
		Cause:     cause,
		Retryable: false, // Validation errors require user input changes
	}
}

// NewAnalysisError creates a new analysis-related error.
func NewAnalysisError(operation, message string, cause error) *AppError {
	return &AppError{
		Type:      ErrorTypeAnalysis,
		Message:   message,
		Operation: operation,
		Cause:     cause,
		Retryable: true, // Analysis might succeed with updated data
	}
}

// NewNetworkError creates a new network-related error.
func NewNetworkError(operation, message string, cause error) *AppError {
	return &AppError{
		Type:      ErrorTypeNetwork,
		Message:   message,
		Operation: operation,
		Cause:     cause,
		Retryable: true, // Network issues are often temporary
	}
}

// NewPermissionError creates a new permission-related error.
func NewPermissionError(operation, message string, cause error) *AppError {
	return &AppError{
		Type:      ErrorTypePermission,
		Message:   message,
		Operation: operation,
		Cause:     cause,
		Retryable: false, // Permission issues require configuration changes
	}
}

// IsConfigError returns true if the error is a configuration error.
func IsConfigError(err error) bool {
	if appErr, ok := err.(*AppError); ok {
		return appErr.Type == ErrorTypeConfig
	}
	return false
}

// IsSLURMError returns true if the error is a SLURM-related error.
func IsSLURMError(err error) bool {
	if appErr, ok := err.(*AppError); ok {
		return appErr.Type == ErrorTypeSLURM
	}
	return false
}

// IsAWSError returns true if the error is an AWS-related error.
func IsAWSError(err error) bool {
	if appErr, ok := err.(*AppError); ok {
		return appErr.Type == ErrorTypeAWS
	}
	return false
}

// IsRetryable returns true if the error might succeed on retry.
func IsRetryable(err error) bool {
	if appErr, ok := err.(*AppError); ok {
		return appErr.Retryable
	}
	return false
}

// WrapError wraps an existing error with additional context.
func WrapError(err error, operation, message string) error {
	if err == nil {
		return nil
	}

	// If it's already an AppError, wrap it
	if appErr, ok := err.(*AppError); ok {
		return &AppError{
			Type:      appErr.Type,
			Message:   fmt.Sprintf("%s: %s", message, appErr.Message),
			Operation: operation,
			Cause:     appErr,
			Retryable: appErr.Retryable,
		}
	}

	// Create new AppError for unknown error types
	return &AppError{
		Type:      ErrorTypeAnalysis, // Default type
		Message:   message,
		Operation: operation,
		Cause:     err,
		Retryable: true, // Be optimistic about unknown errors
	}
}