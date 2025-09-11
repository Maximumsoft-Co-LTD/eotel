package eotel

// Exporter คือ interface สำหรับส่ง log/error ออกภายนอก
type Exporter interface {
	Send(level string, msg string, traceID string, spanID string)
	CaptureError(err error, tags map[string]string, extras map[string]any)
}

// MultiExporter ส่งต่อให้ exporters หลายตัว
type MultiExporter struct{ exporters []Exporter }

func NewMultiExporter(exps ...Exporter) *MultiExporter {
	outs := make([]Exporter, 0, len(exps))
	for _, e := range exps {
		if e != nil {
			outs = append(outs, e)
		}
	}
	return &MultiExporter{exporters: outs}
}

func (m *MultiExporter) Send(level, msg, traceID, spanID string) {
	for _, e := range m.exporters {
		e.Send(level, msg, traceID, spanID)
	}
}

func (m *MultiExporter) CaptureError(err error, tags map[string]string, extras map[string]any) {
	for _, e := range m.exporters {
		e.CaptureError(err, tags, extras)
	}
}

// LokiExporter: ใช้ helper SendLokiAsync
type LokiExporter struct{}

func (LokiExporter) Send(level, msg, traceID, spanID string) {
	SendLokiAsync(level, msg, traceID, spanID)
}
func (LokiExporter) CaptureError(err error, tags map[string]string, extras map[string]any) {
	// optional: จะส่งเข้า Loki ในช่องทาง log ระดับ error ก็ได้
	if err != nil {
		SendLokiAsync("error", err.Error(), "", "")
	}
}

// SentryExporter: ใช้ sentry.CaptureException
type SentryExporter struct{}

func (SentryExporter) Send(level, msg, traceID, spanID string) {
	// ปกติ log ทั่วไปไม่ต้องส่งเข้า Sentry
}
func (SentryExporter) CaptureError(err error, tags map[string]string, extras map[string]any) {
	CaptureError(err, tags, extras)
}
