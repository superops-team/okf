package okf

import "fmt"

// Common errors
var (
	ErrEmptyType   = &ParseError{Message: "type field is required"}
	ErrEmptyTitle  = &ParseError{Message: "title field is required"}
	ErrInvalidPath = &ParseError{Message: "invalid file path"}
)

// ParseError represents an error during parsing with context about the file.
type ParseError struct {
	FilePath string
	Line     int
	Message  string
	Err      error // underlying error if any
}

func (e *ParseError) Error() string {
	if e.FilePath != "" {
		return fmt.Sprintf("%s:%d: %s", e.FilePath, e.Line, e.Message)
	}
	return e.Message
}

func (e *ParseError) Unwrap() error {
	return e.Err
}

// NewParseError creates a new parse error.
func NewParseError(filePath string, line int, msg string) *ParseError {
	return &ParseError{
		FilePath: filePath,
		Line:    line,
		Message: msg,
	}
}

// Wrap adds context to an existing error.
func (e *ParseError) Wrap(msg string) *ParseError {
	return &ParseError{
		FilePath: e.FilePath,
		Line:     e.Line,
		Message:  fmt.Sprintf("%s: %s", msg, e.Message),
		Err:      e,
	}
}
