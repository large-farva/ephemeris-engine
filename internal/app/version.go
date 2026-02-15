package app

// Build-time variables set via -ldflags. For example:
//
//	go build -ldflags "-X github.com/large-farva/ephemeris-engine/internal/app.Version=v1.0.0"
var (
	Version   = "dev"
	GoVersion = "unknown"
	BuiltAt   = "unknown"
)
