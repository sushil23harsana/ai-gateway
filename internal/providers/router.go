package providers

import "strings"

// Router selects a provider for a requested model.
type Router struct {
	byName          map[string]Provider
	modelProvider   map[string]string // model → provider name (from pricing.yaml)
	defaultProvider string
}

// NewRouter builds a router. modelProvider is the model→provider registry
// (typically config.Pricing.ProviderMap()); defaultProvider is the fallback when
// a model can't otherwise be routed.
func NewRouter(provs []Provider, modelProvider map[string]string, defaultProvider string) *Router {
	byName := make(map[string]Provider, len(provs))
	for _, p := range provs {
		byName[p.Name()] = p
	}
	if defaultProvider == "" {
		defaultProvider = "openai"
	}
	if modelProvider == nil {
		modelProvider = map[string]string{}
	}
	return &Router{byName: byName, modelProvider: modelProvider, defaultProvider: defaultProvider}
}

// ByName returns a registered provider by name.
func (r *Router) ByName(name string) (Provider, bool) {
	p, ok := r.byName[name]
	return p, ok
}

// ProviderFor picks the provider for a model: the explicit pricing mapping
// first, then a name-prefix heuristic, then the default provider.
func (r *Router) ProviderFor(model string) (Provider, bool) {
	if name, ok := r.modelProvider[model]; ok {
		if p, ok := r.byName[name]; ok {
			return p, true
		}
	}
	switch m := strings.ToLower(model); {
	case strings.HasPrefix(m, "claude"):
		if p, ok := r.byName["anthropic"]; ok {
			return p, true
		}
	case strings.HasPrefix(m, "gpt"), strings.HasPrefix(m, "o1"), strings.HasPrefix(m, "o3"),
		strings.HasPrefix(m, "o4"), strings.HasPrefix(m, "chatgpt"):
		if p, ok := r.byName["openai"]; ok {
			return p, true
		}
	}
	p, ok := r.byName[r.defaultProvider]
	return p, ok
}
