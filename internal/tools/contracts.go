package tools

import "myclaw/internal/permissions"

// ContractOptions controls which runtime-visible tool contracts are exported.
type ContractOptions struct {
	Policy          permissions.Policy
	IncludeDeferred bool
}

// Contract is the client-neutral runtime description of a tool capability.
type Contract struct {
	Name        string
	Aliases     []string
	Description string
	InputSchema map[string]any
	Source      string
	SearchHint  string
	Enabled     bool
	ReadOnly    bool
	Destructive bool
	ShouldDefer bool
	AlwaysLoad  bool
}

// Contracts returns stable runtime tool contracts for non-UI consumers.
func (r *Registry) Contracts(opts ContractOptions) []Contract {
	defs := r.Expose(ExposeOptions{
		IncludeDeferred: opts.IncludeDeferred,
		Policy:          opts.Policy,
	})
	out := make([]Contract, 0, len(defs))
	for _, def := range defs {
		out = append(out, contractFromDefinition(def))
	}
	return out
}

func contractFromDefinition(def Definition) Contract {
	def = normalizeDefinition(def)
	return Contract{
		Name:        def.Name,
		Aliases:     append([]string(nil), def.Aliases...),
		Description: def.Description,
		InputSchema: deepCloneAnyMap(def.InputSchema),
		Source:      def.Source,
		SearchHint:  def.SearchHint,
		Enabled:     def.Enabled,
		ReadOnly:    def.ReadOnly,
		Destructive: def.Destructive,
		ShouldDefer: def.ShouldDefer,
		AlwaysLoad:  def.AlwaysLoad,
	}
}
