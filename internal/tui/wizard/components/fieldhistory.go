package components

import (
	"strings"
	"sync"
)

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

// NewFieldHistoryFrom restores safe values without sharing the input map.
func NewFieldHistoryFrom(values map[string][]string) *FieldHistory {
	h := NewFieldHistory(8)
	for id, entries := range values {
		for i := len(entries) - 1; i >= 0; i-- {
			h.add(id, entries[i])
		}
	}
	return h
}

func (h *FieldHistory) add(id, value string) {
	if h == nil || !safeHistoryID(id) || !safeHistoryValue(value) {
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

// Record adds a safe value to the front of its field's history.
func (h *FieldHistory) Record(id, value string) { h.add(id, value) }

// Values returns a copy of a field's history, newest first.
func (h *FieldHistory) Values(id string) []string { return h.get(id) }

// Snapshot returns a copy of safe histories for persistence.
func (h *FieldHistory) Snapshot() map[string][]string {
	if h == nil {
		return nil
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	out := make(map[string][]string, len(h.values))
	for id, values := range h.values {
		if safeHistoryID(id) {
			out[id] = append([]string(nil), values...)
		}
	}
	return out
}

func (h *FieldHistory) get(id string) []string {
	if h == nil || !safeHistoryID(id) {
		return nil
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]string(nil), h.values[id]...)
}

func safeHistoryID(id string) bool {
	lower := strings.ToLower(id)
	for _, fragment := range []string{"password", "passwd", "secret", "token", "credential", "username", "api_key", "private_key", "privatekey", "ssh_key", "sshkey", "key_material"} {
		if strings.Contains(lower, fragment) {
			return false
		}
	}
	for _, part := range strings.FieldsFunc(lower, func(r rune) bool { return r == '/' || r == '_' || r == '-' || r == '.' }) {
		if part == "key" {
			return false
		}
	}
	return id != ""
}

func safeHistoryValue(value string) bool {
	trimmed := strings.TrimSpace(value)
	lower := strings.ToLower(trimmed)
	if trimmed == "" || strings.HasPrefix(lower, "-----begin ") || strings.HasPrefix(trimmed, "ssh-rsa ") ||
		strings.HasPrefix(trimmed, "ssh-ed25519 ") || strings.HasPrefix(trimmed, "ecdsa-sha2-") {
		return false
	}
	for _, fragment := range []string{"password=", "passwd=", "token=", "secret=", "private_key=", "authorization:"} {
		if strings.Contains(lower, fragment) {
			return false
		}
	}
	return true
}
