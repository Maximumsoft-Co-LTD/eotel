package eotel

type Config struct {
	ServiceName   string
	JobName       string
	OtelCollector string

	EnableTracing bool
	EnableMetrics bool
	EnableSentry  bool
	EnableLoki    bool

	OTLPUseTLS bool

	SentryDSN string
	LokiURL   string
}

var globalCfg Config

// ถูกประกอบใน InitEOTEL ตาม config (เช่น Loki/Sentry)
var defaultExporter Exporter
