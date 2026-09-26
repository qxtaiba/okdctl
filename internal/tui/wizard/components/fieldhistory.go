package components

import "sync"

// FieldHistory keeps a bounded list of prior non-secret field values in memory.
type FieldHistory struct {
	mu     sync.Mutex
	limit  int
	values map[string][]string
}

// NewFieldHistory creates a history store with a per-field entry limit.
func NewFieldHistory(limit int) *FieldHistory {
	if limit < 1 {
		limit = 1
	}
	return &FieldHistory{limit: limit, values: make(map[string][]string)}
}

func (h *FieldHistory) add(id, value string) {
	if h == nil || id == "" || value == "" {
		return
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	values := h.values[id]
	if len(values) > 0 && values[0] == value {
		return
	}
	values = append([]string{value}, values...)
	if len(values) > h.limit {
		values = values[:h.limit]
	}
	h.values[id] = values
}

func (h *FieldHistory) get(id string) []string {
	if h == nil || id == "" {
		return nil
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]string(nil), h.values[id]...)
}
