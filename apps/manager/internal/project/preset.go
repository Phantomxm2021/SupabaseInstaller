package project

func ApplyPreset(preset Preset) Services {
	services := Services{
		Database:     true,
		Gateway:      true,
		Auth:         true,
		REST:         true,
		Studio:       true,
		PostgresMeta: true,
	}
	switch preset {
	case PresetStandard:
		services.Realtime = true
		services.Storage = true
		services.Functions = true
		services.Supavisor = true
	case PresetFull:
		services.Realtime = true
		services.Storage = true
		services.Imgproxy = true
		services.Functions = true
		services.Supavisor = true
		services.Logs = true
		services.Vector = true
	}
	return services
}
