package acp

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
