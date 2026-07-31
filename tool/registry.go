package tool

// Registry collects tools by name, in registration order.
type Registry struct {
	tools map[string]Tool
	order []string
}

// NewRegistry builds a Registry from a set of tools.
func NewRegistry(tools ...Tool) *Registry {
	r := &Registry{tools: make(map[string]Tool, len(tools))}
	for _, t := range tools {
		r.Add(t)
	}
	return r
}

// Add registers a tool, overwriting any previous tool with the same name.
func (r *Registry) Add(t Tool) {
	if r.tools == nil {
		r.tools = make(map[string]Tool)
	}
	if _, exists := r.tools[t.Name()]; !exists {
		r.order = append(r.order, t.Name())
	}
	r.tools[t.Name()] = t
}

// Get looks up a tool by name.
func (r *Registry) Get(name string) (Tool, bool) {
	t, ok := r.tools[name]
	return t, ok
}

// List returns all registered tools in registration order.
func (r *Registry) List() []Tool {
	out := make([]Tool, 0, len(r.order))
	for _, name := range r.order {
		out = append(out, r.tools[name])
	}
	return out
}

// Names lists every registered tool name.
func (r *Registry) Names() []string {
	out := make([]string, len(r.order))
	copy(out, r.order)
	return out
}
