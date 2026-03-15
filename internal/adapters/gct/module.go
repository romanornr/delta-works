package gct

import "go.uber.org/fx"

// Module provides the GCT-backed exchange adapter layer
var Module = fx.Module("gct", fx.Provide(NewEngine, NewRegistry))
