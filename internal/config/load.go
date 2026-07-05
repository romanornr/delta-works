package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
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
		"outbox.interval":   "500ms",
		"outbox.batch":      100,
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
	if err := resolveSecretFiles(cfg.Venues); err != nil {
		return Config{}, err
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, fmt.Errorf("invalid config: %w", err)
	}
	return cfg, nil
}

// APIAddr returns the control-plane address from the config file, or ""
// when the file or key is absent. Clients use it to find the daemon
// without needing a complete, validated daemon configuration. Errors other
// than a missing file (unreadable, invalid YAML) are returned so they are
// not mistaken for an unconfigured address.
func APIAddr(path string) (string, error) {
	k := koanf.New(".")
	if err := k.Load(file.Provider(path), yaml.Parser()); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "", nil
		}
		return "", fmt.Errorf("load config file %s: %w", path, err)
	}
	return k.String("api.addr"), nil
}

// resolveSecretFiles reads secret files into APIKey and APISecret so the
// rest of the application only sees resolved values. Disabled venues are
// skipped: their secret files may not exist.
func resolveSecretFiles(venues map[string]Venue) error {
	for name, v := range venues {
		if !v.Enabled {
			continue
		}
		var err error
		if v.APIKey, err = resolveSecret(v.APIKey, v.APIKeyFile); err != nil {
			return fmt.Errorf("venues.%s.api_key: %w", name, err)
		}
		if v.APISecret, err = resolveSecret(v.APISecret, v.APISecretFile); err != nil {
			return fmt.Errorf("venues.%s.api_secret: %w", name, err)
		}
		venues[name] = v
	}
	return nil
}

func resolveSecret(value, path string) (string, error) {
	if path == "" {
		return value, nil
	}
	if value != "" {
		return "", errors.New("set either the value or the file, not both")
	}
	if home, ok := strings.CutPrefix(path, "~/"); ok {
		dir, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("resolve ~: %w", err)
		}
		path = filepath.Join(dir, home)
	}
	b, err := os.ReadFile(path) //nolint:gosec // the operator chooses the secret file path
	if err != nil {
		return "", fmt.Errorf("read secret file: %w", err)
	}
	secret := strings.TrimSpace(string(b))
	if secret == "" {
		return "", fmt.Errorf("secret file %s is empty", path)
	}
	return secret, nil
}
