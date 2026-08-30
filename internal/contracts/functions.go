package contracts

import (
	"errors"
	"regexp"
	"time"
)

var functionNamePattern = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)

var ErrInvalidFunctionName = errors.New("invalid function name")

// ValidateFunctionName accepts the URL-safe, directory-safe names supported
// by self-hosted Edge Functions. The main function is the runtime dispatcher
// and must never be managed as user code.
func ValidateFunctionName(name string) error {
	if name == "main" || !functionNamePattern.MatchString(name) {
		return ErrInvalidFunctionName
	}
	return nil
}

// FunctionRelease is the safe release metadata returned to Manager clients.
// It intentionally excludes filesystem locations and user source content.
type FunctionRelease struct {
	SHA256      string    `json:"sha256"`
	OperationID string    `json:"operationId"`
	DeployedAt  time.Time `json:"deployedAt"`
}

// FunctionSummary describes the two releases eligible for management.
type FunctionSummary struct {
	Name     string           `json:"name"`
	Current  *FunctionRelease `json:"current,omitempty"`
	Previous *FunctionRelease `json:"previous,omitempty"`
}
