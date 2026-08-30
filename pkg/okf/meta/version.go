package meta

// Version information
const (
	Version   = "0.3.0"
	BuildDate = "2026-08-30"
)

// Info returns version information as a string.
func Info() string {
	return Version
}
