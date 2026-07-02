// Package ports defines the hexagon boundary: interfaces the application
// depends on, implemented by adapters. Application code never imports
// adapter packages directly.
package ports

import "context"

// HealthChecker reports whether a dependency is usable. Implementations are
// registered with the telemetry server and aggregated by /readyz.
type HealthChecker interface {
	Name() string
	Check(ctx context.Context) error
}
