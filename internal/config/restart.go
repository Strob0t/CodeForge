package config

import (
	"reflect"
	"slices"
	"strings"
)

// ChangedSinceStart re-reads the configuration the way LoadWithCLI read it at
// startup (the same YAML file, environment variables, *_FILE secrets and CLI
// flags) and returns the settings whose value differs from running, as sorted
// dotted YAML keys such as "postgres.dsn".
//
// Services copy their settings when they are built, so a changed setting takes
// effect only after a restart; SIGHUP reloads just the secrets vault. A JWT
// secret generated at startup counts as unchanged while none is configured.
// The names never carry values, so they are safe to log. An invalid
// configuration is returned as an error; running is never modified.
func ChangedSinceStart(running *Config, flags CLIFlags) ([]string, error) {
	current, _, err := loadWithCLI(flags, func(cfg *Config) error {
		if cfg.Auth.JWTSecret == "" && running.Auth.jwtSecretGenerated {
			cfg.Auth.JWTSecret = running.Auth.JWTSecret
			cfg.Auth.jwtSecretGenerated = true
			return nil
		}
		return ensureSecrets(cfg)
	})
	if err != nil {
		return nil, err
	}
	return changedSettings(running, current), nil
}

// configPackage is the package of the Config section types: nested structs
// from it are compared field by field, all other values as a whole.
var configPackage = reflect.TypeFor[Config]().PkgPath()

// changedSettings returns the sorted dotted YAML keys of the exported
// settings that differ between a and b.
func changedSettings(a, b *Config) []string {
	var changed []string
	appendChanged(&changed, "", reflect.ValueOf(a).Elem(), reflect.ValueOf(b).Elem())
	slices.Sort(changed)
	return changed
}

func appendChanged(changed *[]string, prefix string, a, b reflect.Value) {
	for i := range a.NumField() {
		field := a.Type().Field(i)
		if !field.IsExported() {
			continue
		}
		key := yamlKey(&field)
		if prefix != "" {
			key = prefix + "." + key
		}
		if field.Type.Kind() == reflect.Struct && field.Type.PkgPath() == configPackage {
			appendChanged(changed, key, a.Field(i), b.Field(i))
			continue
		}
		if !reflect.DeepEqual(a.Field(i).Interface(), b.Field(i).Interface()) {
			*changed = append(*changed, key)
		}
	}
}

// yamlKey is the name a setting has in the YAML file.
func yamlKey(field *reflect.StructField) string {
	name, _, _ := strings.Cut(field.Tag.Get("yaml"), ",")
	if name == "" {
		return strings.ToLower(field.Name)
	}
	return name
}
