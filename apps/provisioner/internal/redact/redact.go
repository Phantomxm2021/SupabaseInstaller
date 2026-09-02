package redact

import (
	"supabase-manager/internal/diagnostic"
)

const marker = "[REDACTED]"

type Redactor struct {
	known []string
}

func New(knownValues []string) *Redactor {
	values := make([]string, 0, len(knownValues))
	for _, value := range knownValues {
		if len(value) >= 4 {
			values = append(values, value)
		}
	}
	return &Redactor{known: values}
}

func (r *Redactor) String(input string) string {
	return diagnostic.Sanitize(input, r.known)
}
