package eventworker

import _ "embed"

//go:embed index.ts
var source []byte

// Source returns a copy of the Manager-owned Edge Runtime event adapter.
func Source() []byte { return append([]byte(nil), source...) }
