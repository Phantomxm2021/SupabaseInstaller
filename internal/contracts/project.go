package contracts

import "time"

type ProjectStatus string

const (
	ProjectStatusDraft      ProjectStatus = "DRAFT"
	ProjectStatusInstalling ProjectStatus = "INSTALLING"
	ProjectStatusRunning    ProjectStatus = "RUNNING"
	ProjectStatusStopped    ProjectStatus = "STOPPED"
	ProjectStatusDegraded   ProjectStatus = "DEGRADED"
	ProjectStatusFailed     ProjectStatus = "FAILED"
	ProjectStatusDeleting   ProjectStatus = "DELETING"
)

type HealthStatus string

const (
	HealthHealthy   HealthStatus = "HEALTHY"
	HealthDegraded  HealthStatus = "DEGRADED"
	HealthStarting  HealthStatus = "STARTING"
	HealthStopped   HealthStatus = "STOPPED"
	HealthUnhealthy HealthStatus = "UNHEALTHY"
	HealthUnknown   HealthStatus = "UNKNOWN"
)

type Preset string

const (
	PresetLightweight Preset = "LIGHTWEIGHT"
	PresetStandard    Preset = "STANDARD"
	PresetFull        Preset = "FULL"
	PresetCustom      Preset = "CUSTOM"
)

type Services struct {
	Database     bool `json:"database"`
	Gateway      bool `json:"gateway"`
	Auth         bool `json:"auth"`
	REST         bool `json:"rest"`
	Studio       bool `json:"studio"`
	PostgresMeta bool `json:"postgresMeta"`
	Realtime     bool `json:"realtime"`
	Storage      bool `json:"storage"`
	Imgproxy     bool `json:"imgproxy"`
	Functions    bool `json:"functions"`
	Supavisor    bool `json:"supavisor"`
	Logs         bool `json:"logs"`
	Vector       bool `json:"vector"`
	DirectDB     bool `json:"directDb"`
}

type ProjectDraft struct {
	Name            string   `json:"name"`
	Slug            string   `json:"slug"`
	Domain          string   `json:"domain"`
	SiteURL         string   `json:"siteUrl"`
	SupabaseVersion string   `json:"supabaseVersion"`
	Preset          Preset   `json:"preset"`
	Services        Services `json:"services"`
}

type Project struct {
	ID              string        `json:"id"`
	Name            string        `json:"name"`
	Slug            string        `json:"slug"`
	Domain          string        `json:"domain"`
	SiteURL         string        `json:"siteUrl"`
	Status          ProjectStatus `json:"status"`
	Health          HealthStatus  `json:"health"`
	SupabaseVersion string        `json:"supabaseVersion"`
	Preset          Preset        `json:"preset"`
	Services        Services      `json:"services"`
	CreatedAt       time.Time     `json:"createdAt"`
	UpdatedAt       time.Time     `json:"updatedAt"`
}
