// Package agentconfig owns deterministic, project-scoped planning of AI agent
// integration files (MCP servers and workflow guidance) for Cursor, Claude
// Code and Codex. It never calls query, MCP, embedding or index logic; it only
// reasons about repository files under a self-describing ownership model.
package agentconfig

// Stable machine-readable error codes for the agent configuration surface.
const (
	// ErrAgentConfigConflict means ownership is ambiguous or the host file
	// cannot be safely parsed. Nothing is overwritten.
	ErrAgentConfigConflict = "agent_config_conflict"
	// ErrAgentConfigDrifted means a recognized OKF-owned block/file is
	// syntactically valid but differs from the deterministic render.
	ErrAgentConfigDrifted = "agent_config_drifted"
	// ErrUnsupportedAgentClient means the client value is unknown or a known
	// adapter version is not supported.
	ErrUnsupportedAgentClient = "unsupported_agent_client"
	// ErrUnsafeAgentConfigPath means a selected path escapes the repository
	// root (.., absolute path, or symlink escape).
	ErrUnsafeAgentConfigPath = "unsafe_agent_config_path"
	// ErrRequiresConfirmation is returned by mutating operations in
	// non-interactive mode without an explicit yes.
	ErrRequiresConfirmation = "agent_config_confirmation_required"
)

// OwnedMarkerKey is the non-secret JSON env key that self-describes OKF
// ownership of an mcpServers entry. Ownership requires the exact value below.
const (
	OwnedMarkerKey   = "OKF_MANAGED"
	OwnedMarkerValue = "agentconfig-v1"
)

// PendingFileHeader is the whole-file ownership header emitted at the top of
// every OKF-created rule/skill file. A pre-existing file is considered owned
// only when it contains this exact marker.
const PendingFileHeader = "<!-- OKF-MANAGED: agentconfig-v1 -->"

// marker blocks for the delimited TOML and Markdown adapters.
const (
	tomlBegin = "# BEGIN OKF MANAGED MCP v1"
	tomlEnd   = "# END OKF MANAGED MCP v1"
	agBegin   = "<!-- BEGIN OKF MANAGED AGENT v1 -->"
	agEnd     = "<!-- END OKF MANAGED AGENT v1 -->"
)

// AgentConfigError carries a stable code so the CLI can print it verbatim.
type AgentConfigError struct {
	Code        string
	Message     string
	Remediation string
	Path        string
}

func (e *AgentConfigError) Error() string {
	if e.Path != "" {
		return e.Code + ": " + e.Message + " (" + e.Path + ")"
	}
	return e.Code + ": " + e.Message
}

func errConflict(message string) *AgentConfigError {
	return &AgentConfigError{
		Code:        ErrAgentConfigConflict,
		Message:     message,
		Remediation: "Resolve or remove the unowned/ambiguous OKF entry manually; OKF never overwrites it.",
	}
}

func errUnsupported(message string) *AgentConfigError {
	return &AgentConfigError{
		Code:        ErrUnsupportedAgentClient,
		Message:     message,
		Remediation: "Use --client cursor|claude-code|codex|all.",
	}
}

func errUnsafePath(message string) *AgentConfigError {
	return &AgentConfigError{
		Code:        ErrUnsafeAgentConfigPath,
		Message:     message,
		Remediation: "Keep all adapter paths inside the selected repository root.",
	}
}
