package registry

import "encoding/json"

// CapabilitySchema describes what an agent can do and its input/output contract.
type CapabilitySchema struct {
	// Name is the capability identifier (usually Agentfile NAME)
	Name string `json:"name"`

	// Version for compatibility checking (semver recommended)
	Version string `json:"version,omitempty"`

	// Description is human-readable explanation
	Description string `json:"description,omitempty"`

	// Inputs describes required and optional input parameters
	Inputs []FieldSchema `json:"inputs,omitempty"`

	// Outputs describes what the capability produces
	Outputs []FieldSchema `json:"outputs,omitempty"`
}

// FieldSchema describes an input or output field.
type FieldSchema struct {
	// Name is the field identifier
	Name string `json:"name"`

	// Required indicates if the field must be provided
	Required bool `json:"required"`

	// Default value if not provided (empty string if no default)
	Default string `json:"default,omitempty"`

	// Type hint: "string", "number", "boolean", "json"
	Type string `json:"type,omitempty"`

	// Description is human-readable explanation
	Description string `json:"description,omitempty"`
}

// ServiceAgentInfo extends AgentInfo with capability schema for service agents.
type ServiceAgentInfo struct {
	AgentInfo

	// Capability this service agent provides (one per agent)
	Capability CapabilitySchema `json:"capability"`

	// NodeID identifies the machine/container running this agent
	NodeID string `json:"node_id,omitempty"`

	// Version of the agent software
	AgentVersion string `json:"agent_version,omitempty"`
}

// Validate checks if the capability schema is valid.
func (c *CapabilitySchema) Validate() error {
	if c.Name == "" {
		return ErrInvalidID
	}
	return nil
}

// Marshal serializes the capability schema to JSON.
func (c *CapabilitySchema) Marshal() ([]byte, error) {
	return json.Marshal(c)
}

// UnmarshalCapabilitySchema deserializes a capability schema from JSON.
func UnmarshalCapabilitySchema(data []byte) (*CapabilitySchema, error) {
	var c CapabilitySchema
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, err
	}
	return &c, nil
}

// HasRequiredInput checks if a capability has a specific required input.
func (c *CapabilitySchema) HasRequiredInput(name string) bool {
	for _, input := range c.Inputs {
		if input.Name == name && input.Required {
			return true
		}
	}
	return false
}

// GetInputDefault returns the default value for an input field.
func (c *CapabilitySchema) GetInputDefault(name string) (string, bool) {
	for _, input := range c.Inputs {
		if input.Name == name && input.Default != "" {
			return input.Default, true
		}
	}
	return "", false
}
