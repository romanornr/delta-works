// Command daemon is the trading daemon; the Makefile names the binary
// (bin/$(NAME)d).
package main

import (
	"flag"

	"github.com/romanornr/delta-works/internal/app"
)

func main() {
	configPath := flag.String("config", "config.yaml", "path to configuration file")
	flag.Parse()

	explicit := false
	flag.Visit(func(f *flag.Flag) {
		if f.Name == "config" {
			explicit = true
		}
	})

	app.New(*configPath, explicit).Run()
}
