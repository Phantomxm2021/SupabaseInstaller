package contracts

type SetupStatusResponse struct {
	Required bool `json:"required"`
}

type SetupRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type SetupResponse struct {
	RecoveryCodes []string `json:"recoveryCodes"`
}

type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type SessionResponse struct {
	Username           string `json:"username"`
	MustChangePassword bool   `json:"mustChangePassword"`
	CSRFToken          string `json:"csrfToken"`
}
