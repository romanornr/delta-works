package observability

import "go.uber.org/fx"

// Module provides observability to the Fx dependency injection containers.
var Module = fx.Module("observability", fx.Provide(NewLogger))
