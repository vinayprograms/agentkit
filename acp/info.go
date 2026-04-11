package acp

// Info identifies an agent or client implementation.
type Info struct {
	Name    string `json:"name"`
	Title   string `json:"title,omitempty"`
	Version string `json:"version"`
}

// AgentCaps advertises agent capabilities during initialization.
type AgentCaps struct {
	LoadSession bool       `json:"loadSession,omitempty"`
	Prompt      PromptCaps `json:"promptCapabilities,omitempty"`
	MCP         []string   `json:"mcpTransports,omitempty"` // "stdio", "http"
	Auth        []string   `json:"authMethods,omitempty"`
	Meta        Meta       `json:"_meta,omitempty"`
}

// PromptCaps describes which content types the agent accepts in prompts.
type PromptCaps struct {
	Image           bool `json:"image,omitempty"`
	Audio           bool `json:"audio,omitempty"`
	EmbeddedContext bool `json:"embeddedContext,omitempty"`
}

// ClientCaps advertises host capabilities during initialization.
type ClientCaps struct {
	Terminal      bool `json:"terminal,omitempty"`
	ReadTextFile  bool `json:"fs.readTextFile,omitempty"`
	WriteTextFile bool `json:"fs.writeTextFile,omitempty"`
	Meta          Meta `json:"_meta,omitempty"`
}

// InitParams is sent by the host to begin the handshake.
type InitParams struct {
	ProtocolVersion int        `json:"protocolVersion"`
	Info            Info       `json:"clientInfo"`
	Capabilities    ClientCaps `json:"capabilities"`
	Meta            Meta       `json:"_meta,omitempty"`
}

// InitResult is returned by the agent after negotiation.
type InitResult struct {
	ProtocolVersion int       `json:"protocolVersion"`
	Info            Info      `json:"agentInfo"`
	Capabilities    AgentCaps `json:"capabilities"`
	Meta            Meta      `json:"_meta,omitempty"`
}

// AuthParams is sent by the host to authenticate with the agent.
type AuthParams struct {
	Method      string `json:"method"`
	Credentials any    `json:"credentials,omitempty"`
	Meta        Meta   `json:"_meta,omitempty"`
}

// AuthResult is returned by the agent on successful authentication.
type AuthResult struct {
	Meta Meta `json:"_meta,omitempty"`
}
