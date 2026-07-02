package config

import (
	"errors"
	"fmt"
	"io/fs"
	"strings"

	"github.com/knadh/koanf/parsers/yaml"
	"github.com/knadh/koanf/providers/confmap"
	env "github.com/knadh/koanf/providers/env/v2"
	"github.com/knadh/koanf/providers/file"
	"github.com/knadh/koanf/v2"
)

func defaults() map[string]any {
	return map[string]any{
		"log.level":         "info",
		"log.format":        "console",
		"http.addr":         ":8080",
		"snapshot.interval": "60s",
	}
}

// Load reads configuration in precedence order: defaults < YAML file < env.
// A missing file is fatal only when the path was explicitly provided;
// the default "config.yaml" may be absent (env-only setups).
func Load(path string, explicit bool) (Config, error) {
	k := koanf.New(".")

	if err := k.Load(confmap.Provider(defaults(), "."), nil); err != nil {
		return Config{}, fmt.Errorf("load defaults: %w", err)
	}

	if err := k.Load(file.Provider(path), yaml.Parser()); err != nil {
		if explicit || !errors.Is(err, fs.ErrNotExist) {
			return Config{}, fmt.Errorf("load config file %s: %w", path, err)
		}
	}

	// DELTA__SECTION__KEY=value → section.key. Single underscores survive,
	// so DELTA__VENUES__BYBIT__API_KEY → venues.bybit.api_key.
	// List-valued keys are split on commas here because the env provider
	// hands over one string, which would otherwise land as a single
	// element. Only known list keys are split: values such as DSNs may
	// legitimately contain commas.
	if err := k.Load(env.Provider(".", env.Opt{
		Prefix: EnvPrefix,
		TransformFunc: func(key, value string) (string, any) {
			key = strings.ToLower(strings.TrimPrefix(key, EnvPrefix))
			key = strings.ReplaceAll(key, "__", ".")
			if strings.HasSuffix(key, ".accounts") {
				parts := strings.Split(value, ",")
				for i := range parts {
					parts[i] = strings.TrimSpace(parts[i])
				}
				return key, parts
			}
			return key, value
		},
	}), nil); err != nil {
		return Config{}, fmt.Errorf("load env: %w", err)
	}

	var cfg Config
	if err := k.Unmarshal("", &cfg); err != nil {
		return Config{}, fmt.Errorf("unmarshal config: %w", err)
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, fmt.Errorf("invalid config: %w", err)
	}
	return cfg, nil
}
