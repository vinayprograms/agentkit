package rpc

// JSON-RPC method names defined by the ACP spec.
const (
	MethodInitialize   = "initialize"
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
