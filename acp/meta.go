package acp

// Meta carries extensibility data on protocol types.
// Reserved keys: "traceparent", "tracestate", "baggage" (W3C trace context).
type Meta map[string]any

// JSON-RPC method names.
const (
	MethodInitialize = "initialize"
	MethodAuthenticate = "authenticate"

	MethodSessionNew    = "session/new"
	MethodSessionLoad   = "session/load"
	MethodSessionPrompt = "session/prompt"
	MethodSessionCancel = "session/cancel"
	MethodSessionUpdate = "session/update"

	MethodSetMode   = "session/set_mode"
	MethodSetConfig = "session/set_config_option"

	MethodRequestPermission = "session/request_permission"

	MethodReadFile  = "fs/read_text_file"
	MethodWriteFile = "fs/write_text_file"

	MethodTerminalCreate  = "terminal/create"
	MethodTerminalOutput  = "terminal/output"
	MethodTerminalWait    = "terminal/wait_for_exit"
	MethodTerminalKill    = "terminal/kill"
	MethodTerminalRelease = "terminal/release"
)
