package meta

// Version information
const (
	Version   = "1.0.0"
	BuildDate = "2026-06-13"
)

// Info returns version information as a string.
func Info() string {
	return Version
}
