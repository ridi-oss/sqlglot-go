package schema

type Mapping struct {
	keys []string
	m    map[string]any
}

func NewMapping() *Mapping {
	return &Mapping{m: map[string]any{}}
}

func M(pairs ...any) *Mapping {
	mapping := NewMapping()
	for i := 0; i+1 < len(pairs); i += 2 {
		key, _ := pairs[i].(string)
		mapping.Set(key, pairs[i+1])
	}
	return mapping
}

func (m *Mapping) Set(k string, v any) {
	if m.m == nil {
		m.m = map[string]any{}
	}
	if _, ok := m.m[k]; !ok {
		m.keys = append(m.keys, k)
	}
	m.m[k] = v
}

func (m *Mapping) Get(k string) (any, bool) {
	if m == nil || m.m == nil {
		return nil, false
	}
	v, ok := m.m[k]
	return v, ok
}

func (m *Mapping) Keys() []string {
	if m == nil {
		return nil
	}
	out := make([]string, len(m.keys))
	copy(out, m.keys)
	return out
}

func (m *Mapping) Len() int {
	if m == nil {
		return 0
	}
	return len(m.keys)
}

func (m *Mapping) first() any {
	if m == nil || len(m.keys) == 0 {
		return nil
	}
	return m.m[m.keys[0]]
}

func (m *Mapping) Copy() *Mapping {
	if m == nil {
		return nil
	}
	out := NewMapping()
	out.keys = append(out.keys, m.keys...)
	out.m = make(map[string]any, len(m.m))
	for k, v := range m.m {
		out.m[k] = v
	}
	return out
}
