// mod_remediation.go — surface the remediation registry as MCP tools.
//
// The registry maps stable error codes to structured fixes. These tools
// are the read-side: callers look up a single remediation or browse the
// full catalog. Fixes themselves are dispatched by invoking the referenced
// tool separately — this module does not execute them.
package dotfiles

import (
	"context"
	"fmt"

	"github.com/hairglasses-studio/dotfiles-mcp/internal/remediation"
	"github.com/hairglasses-studio/mcpkit/handler"
	"github.com/hairglasses-studio/mcpkit/registry"
)

// RemediationLookupInput selects a single remediation by its error code.
type RemediationLookupInput struct {
	Code string `json:"code" jsonschema:"required,description=Error code registered in the remediation catalog (see remediation_list)"`
}

// RemediationLookupOutput is the lookup response. Found=false when the code
// is not registered; in that case Remediation is the zero value.
type RemediationLookupOutput struct {
	Found       bool                    `json:"found"`
	Code        string                  `json:"code"`
	Remediation remediation.Remediation `json:"remediation,omitempty"`
}

// RemediationListOutput is the full catalog response.
type RemediationListOutput struct {
	Count   int                 `json:"count"`
	Entries []remediation.Entry `json:"entries"`
}

// RemediationModule exposes the remediation catalog as read-only tools.
type RemediationModule struct{}

func (m *RemediationModule) Name() string { return "remediation" }
func (m *RemediationModule) Description() string {
	return "Structured error → fix registry. Returns callable Remediation records that consumers can dispatch directly instead of reinterpreting free-form suggestion strings."
}

func (m *RemediationModule) Tools() []registry.ToolDefinition {
	return []registry.ToolDefinition{
		handler.TypedHandler[RemediationLookupInput, RemediationLookupOutput](
			"remediation_lookup",
			"Look up the structured remediation for an error code. Returns Found=false when the code is not registered. Consumers should dispatch the referenced Tool with Args to apply the fix.",
			func(_ context.Context, input RemediationLookupInput) (RemediationLookupOutput, error) {
				if input.Code == "" {
					return RemediationLookupOutput{}, fmt.Errorf("[%s] code is required", handler.ErrInvalidParam)
				}
				rem, ok := remediation.Lookup(remediation.ErrorCode(input.Code))
				return RemediationLookupOutput{
					Found:       ok,
					Code:        input.Code,
					Remediation: rem,
				}, nil
			},
		),
		handler.TypedHandler[EmptyInput, RemediationListOutput](
			"remediation_list",
			"List every registered error code and its structured remediation. Useful for auditing coverage and for UIs that surface known auto-fix paths.",
			func(_ context.Context, _ EmptyInput) (RemediationListOutput, error) {
				entries := remediation.List()
				return RemediationListOutput{
					Count:   len(entries),
					Entries: entries,
				}, nil
			},
		),
	}
}
