package transform

import "fmt"

// Registry 按名字查 Transformer。名字来自 TableMapping.Transform。
type Registry struct {
	m map[string]Transformer
}

func NewRegistry() *Registry {
	return &Registry{m: make(map[string]Transformer)}
}

// Register 注册一个具名 Transformer（重名覆盖）。
func (r *Registry) Register(name string, t Transformer) {
	r.m[name] = t
}

// Get 按名取 Transformer：空名返 Identity（未指定 transform 的表原样透传）；
// 非空但未注册返 error（暴露配置错误，不静默退化成 Identity）。
func (r *Registry) Get(name string) (Transformer, error) {
	if name == "" {
		return IdentityTransformer{}, nil
	}
	t, ok := r.m[name]
	if !ok {
		return nil, fmt.Errorf("transform %q not registered", name)
	}
	return t, nil
}
