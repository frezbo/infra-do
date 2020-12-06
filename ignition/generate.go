package ignition

import (
	"encoding/json"

	"github.com/frezbo/infra-do/template"
)

const (
	ignitionConfigVersion = "2.1.0"
	boolTruePtr           = &[]bool{true}[0]
)

func generateConfig(systemDConfig template.SystemDConfig) (Config, error) {
	config := Config{
		Ignition: Ignition{
			Version: ignitionConfigVersion,
		},
	}
	var units []Unit
	for name, systemDUnit := range systemDConfig {
		systemDUnitContent, err := template.RenderSystemDUnit(systemDUnit)
		if err != nil {
			return Config{}, err
		}
		units = append(units, Unit{
			Name:     name,
			Enabled:  boolTruePtr,
			Contents: systemDUnitContent,
		})
	}
	config.Systemd = Systemd{
		Units: units,
	}
	return config, nil
}

func toJSON(config Config) (string, error) {
	bytes, err := json.MarshalIndent(config, "", "    ")
	if err != nil {
		return "", nil
	}
	return string(bytes), nil
}
