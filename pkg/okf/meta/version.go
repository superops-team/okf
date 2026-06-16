package meta

// Version information
const (
	Version   = "1.2.0"
	BuildDate = "2026-06-16"
)

// Info returns version information as a string.
func Info() string {
	return Version
}
