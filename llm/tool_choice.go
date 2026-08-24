package llm

// toolChoiceMode is the internal discriminator for ToolChoice. It is
// unexported: callers build a ToolChoice via ToolChoiceAuto, ToolChoiceRequired,
// or ToolChoiceTool, mirroring the ThinkingLevel per-call-override style.
type toolChoiceMode string

const (
	toolChoiceModeAuto     toolChoiceMode = ""
	toolChoiceModeRequired toolChoiceMode = "required"
	toolChoiceModeTool     toolChoiceMode = "tool"
)

// ToolChoice controls whether, and which, tool a model must call for a single
// request. The zero value is ToolChoiceAuto: the model decides on its own
// whether to call a tool. Providers/models that cannot honor a given choice
// must degrade to auto rather than erroring — the caller always keeps a
// fallback (prose parsing, a retry, etc.).
type ToolChoice struct {
	mode toolChoiceMode
	tool string // set only when mode == toolChoiceModeTool
}

// ToolChoiceAuto lets the model decide whether to call a tool. It is the
// zero value of ToolChoice, so an unset ChatRequest.ToolChoice behaves the
// same as this.
var ToolChoiceAuto = ToolChoice{}

// ToolChoiceRequired forces the model to call some tool, without pinning
// which one.
var ToolChoiceRequired = ToolChoice{mode: toolChoiceModeRequired}

// ToolChoiceTool forces the model to call the named tool.
func ToolChoiceTool(name string) ToolChoice {
	return ToolChoice{mode: toolChoiceModeTool, tool: name}
}

// IsAuto reports whether the choice is the auto (zero-value) mode.
func (c ToolChoice) IsAuto() bool { return c.mode == toolChoiceModeAuto }

// IsRequired reports whether the choice requires some tool call, without
// naming one.
func (c ToolChoice) IsRequired() bool { return c.mode == toolChoiceModeRequired }

// ToolName returns the forced tool's name and true when the choice pins a
// specific tool; otherwise ("", false).
func (c ToolChoice) ToolName() (string, bool) {
	if c.mode == toolChoiceModeTool {
		return c.tool, true
	}
	return "", false
}
