package core

import "sort"

type Registry struct{ plugins map[string]ProviderPlugin }

func NewRegistry(ps ...ProviderPlugin) *Registry {
	r := &Registry{plugins: map[string]ProviderPlugin{}}
	for _, p := range ps {
		r.plugins[p.Name()] = p
	}
	return r
}

func (r *Registry) Get(name string) (ProviderPlugin, bool) {
	p, ok := r.plugins[name]
	return p, ok
}

// List returns plugins sorted by name.
func (r *Registry) List() []ProviderPlugin {
	out := make([]ProviderPlugin, 0, len(r.plugins))
	for _, p := range r.plugins {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name() < out[j].Name() })
	return out
}
