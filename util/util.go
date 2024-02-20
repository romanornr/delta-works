package util

import "os"

// ConfigFile returns the path to the configuration file.
// It looks for the file in the given path, then in the GCTCONFIG environment variable,
// and finally in the default location ~/.gocryptotrader/config.json.
// If the file does not exist, it returns the empty string.
func ConfigFile(inp string) string {
	if inp != "" {
		path := ExpandUser(inp)
		if FileExists(path) {
			return path
		}
	}

	if env := os.Getenv("GCTCONFIG"); env != "" {
		path := ExpandUser(env)
		if FileExists(path) {
			return path
		}
	}

	if path := ExpandUser("~/.gocryptotrader/config.json"); FileExists(path) {
		return path
	}

	return inp
}

// ExpandUser expands the username in the given path.
// If the path does not start with a tilde, it is returned unchanged.
// On Windows, it also expands environment variables that start with a percent sign.
func ExpandUser(path string) string {
	if len(path) == 0 || path[0] != '~' {
		return path
	}
	return os.Getenv("HOME") + path[1:]
}

// FileExists returns whether the named file or directory exists or not.
func FileExists(file string) bool {
	if _, err := os.Stat(file); os.IsNotExist(err) {
		return false
	}
	return true
}
