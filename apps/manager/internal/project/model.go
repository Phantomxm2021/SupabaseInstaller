package project

import "supabase-manager/internal/contracts"

type Draft = contracts.ProjectDraft
type Services = contracts.Services
type Preset = contracts.Preset
type Project = contracts.Project
type ProjectStatus = contracts.ProjectStatus
type HealthStatus = contracts.HealthStatus

const (
	PresetLightweight  = contracts.PresetLightweight
	PresetStandard     = contracts.PresetStandard
	PresetFull         = contracts.PresetFull
	PresetCustom       = contracts.PresetCustom
	ProjectStatusDraft = contracts.ProjectStatusDraft
	HealthUnknown      = contracts.HealthUnknown
)
