package meta

// Version information.
// Version is a var (not const) so release builds can override it via
// `-ldflags "-X github.com/superops-team/okf/pkg/okf/meta.Version=vX.Y.Z"`.
var (
	Version   = "0.4.1"
	BuildDate = "2026-08-30"
)

// Info returns version information as a string.
func Info() string {
	return Version
}
