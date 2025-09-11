package eotel

import (
	"context"
	"fmt"
	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/codes"
	"net/http"
	"os"
	"sort"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
)

type loggerCtxKey struct{}

// NOTE: Exporter interface moved to exporter.go

type Eotel struct {
	ctx          context.Context
	logger       *zap.Logger
	tracer       trace.Tracer
	meter        metric.Meter
	span         trace.Span
	logCounter   metric.Int64Counter
	durationHist metric.Float64Histogram
	fields       []zap.Field
	attrs        []attribute.KeyValue
	err          error
	name         string
	start        time.Time
	exporter     Exporter
}

func New(ctx context.Context, name string) *Eotel {
	meter := otel.Meter(globalCfg.ServiceName)
	logCounter, durationHist := initMetrics(meter)
	return &Eotel{
		ctx:          ctx,
		logger:       zap.L(),
		tracer:       otel.Tracer(globalCfg.ServiceName),
		meter:        meter,
		logCounter:   logCounter,
		durationHist: durationHist,
		start:        time.Now(),
		exporter:     defaultExporter, // ใช้ default exporter ที่ประกอบจาก InitEOTEL
		name:         name,
	}
}

// NewWithExporter: ระบุ exporter เอง
func NewWithExporter(ctx context.Context, name string, exp Exporter) *Eotel {
	l := New(ctx, name)
	l.exporter = exp
	return l
}

// SetExporter: เปลี่ยน exporter runtime
func (l *Eotel) SetExporter(exp Exporter) *Eotel {
	l.exporter = exp
	return l
}

func Inject(ctx context.Context, logger *Eotel) context.Context {
	return context.WithValue(ctx, loggerCtxKey{}, logger)
}

func FromContext(ctx context.Context, name string) *Eotel {
	if val := ctx.Value(loggerCtxKey{}); val != nil {
		if lg, ok := val.(*Eotel); ok && lg != nil {
			return lg
		}
	}
	return Noop(name) // fallback safe
}

func FromGin(c *gin.Context, name string) *Eotel {
	return FromContext(c.Request.Context(), name)
}

func RecoverPanic(c *gin.Context) func() {
	return func() {
		if rec := recover(); rec != nil {
			err := fmt.Errorf("panic: %v", rec)

			log := Safe(FromGin(c, "panic")).WithError(err)
			log.Error("unhandled panic")

			span := trace.SpanFromContext(c.Request.Context())
			if span != nil {
				span.RecordError(err)
				span.SetStatus(codes.Error, err.Error())
			}

			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{
				"error": "internal server error",
			})
		}
	}
}

func Safe(l *Eotel) *Eotel {
	if l == nil {
		return Noop("safe")
	}
	return l
}

func (l *Eotel) Info(msg string)  { l.log("info", msg) }
func (l *Eotel) Error(msg string) { l.log("error", msg) }
func (l *Eotel) Debug(msg string) { l.log("debug", msg) }
func (l *Eotel) Warn(msg string)  { l.log("warn", msg) }
func (l *Eotel) Fatal(msg string) {
	l.log("fatal", msg)
	if l.span != nil {
		l.span.End()
	}
	os.Exit(1)
}

func (l *Eotel) log(level, msg string) {
	if l == nil {
		fmt.Printf("[%s] %s\n", level, msg)
		return
	}
	l.startSpanIfNeeded()

	sc := trace.SpanContext{}
	if l.span != nil {
		sc = l.span.SpanContext()
	}

	traceID := sc.TraceID().String()
	fields := append([]zap.Field{
		zap.String("trace_id", traceID),
		zap.String("span_id", sc.SpanID().String()),
		zap.String("job", globalCfg.JobName),
		zap.String("service", globalCfg.ServiceName),
		zap.String("level", level),
	}, l.fields...)

	if l.logger != nil {
		switch level {
		case "info":
			l.logger.Info(msg, fields...)
		case "error":
			l.logger.Error(msg, fields...)
		case "debug":
			l.logger.Debug(msg, fields...)
		case "warn":
			l.logger.Warn(msg, fields...)
		case "fatal":
			l.logger.Fatal(msg, fields...)
		}
	}

	// เปลี่ยนเงื่อนไข: ส่งออกผ่าน exporter "ถ้ามี" (ไม่ผูกกับ EnableLoki)
	if l.exporter != nil {
		l.exporter.Send(level, msg, traceID, sc.SpanID().String())
	}

	l.endSpan(msg, level)
}

func (l *Eotel) TraceName(name string) *Eotel {
	l.name = name
	return l
}

func (l *Eotel) WithField(key string, value any) *Eotel {
	if l == nil {
		return Noop("WithField")
	}
	if key == "" {
		return l
	}
	l.fields = append(l.fields, zap.Any(key, value))
	l.attrs = append(l.attrs, attribute.String(key, fmt.Sprintf("%v", value)))
	return l
}

func (l *Eotel) WithFields(m map[string]any) *Eotel {
	for k, v := range m {
		l.WithField(k, v)
	}
	return l
}

func (l *Eotel) WithError(err error) *Eotel {
	if err != nil {
		l.err = err
		l.fields = append(l.fields, zap.Error(err))
		l.attrs = append(l.attrs, attribute.String("error", err.Error()))
		if l.exporter != nil {
			l.exporter.CaptureError(err, map[string]string{}, map[string]any{"error": err.Error()})
		}
	}
	return l
}

func (l *Eotel) Ctx() context.Context {
	return l.ctx
}

func (l *Eotel) startSpanIfNeeded() {
	if l.span == nil {
		l.ctx, l.span = l.tracer.Start(l.ctx, l.name)
	}
}

func (l *Eotel) Span() trace.Span {
	if l == nil {
		return nil
	}
	return l.span
}

func (l *Eotel) endSpan(msg, level string) {
	if l == nil {
		return
	}
	durationMs := time.Since(l.start).Seconds() * 1000

	l.attrs = append(l.attrs,
		attribute.String("log.message", msg),
		attribute.String("log.level", level),
		attribute.Float64("duration_ms", durationMs),
	)

	sort.SliceStable(l.attrs, func(i, j int) bool {
		return string(l.attrs[i].Key) < string(l.attrs[j].Key)
	})

	if l.span != nil {
		l.span.SetAttributes(l.attrs...)
		if l.err != nil {
			l.span.SetStatus(codes.Error, l.err.Error())
			l.span.RecordError(l.err)
		}
		l.span.End()
	}

	if l.meter != nil {
		l.logCounter.Add(l.ctx, 1, metric.WithAttributes(attribute.String("level", level)))
		l.durationHist.Record(l.ctx, durationMs, metric.WithAttributes(attribute.String("level", level)))
	}
}

func initMetrics(m metric.Meter) (metric.Int64Counter, metric.Float64Histogram) {
	c, _ := m.Int64Counter("log_total")
	h, _ := m.Float64Histogram("log_duration_ms")
	return c, h
}

func (l *Eotel) WithTracer(name string, fn func(ctx context.Context) error) error {
	ctx, span := l.tracer.Start(l.ctx, name)
	defer span.End()
	return fn(ctx)
}

func (l *Eotel) SpanEvent(name string, attrs ...attribute.KeyValue) {
	if l.span != nil {
		l.span.AddEvent(name, trace.WithAttributes(attrs...))
	}
}

func (l *Eotel) SetSpanAttr(key string, value any) {
	if l.span != nil {
		l.span.SetAttributes(attribute.String(key, fmt.Sprintf("%v", value)))
	}
}

func (l *Eotel) SetSpanError(err error) {
	if err != nil && l.span != nil {
		l.span.RecordError(err)
	}
}

func (l *Eotel) Child(name string) *Eotel {
	if l == nil {
		return Noop(name)
	}
	ctx := l.ctx
	tracer := l.tracer
	if tracer == nil {
		tracer = otel.Tracer(globalCfg.ServiceName)
	}
	ctx, span := tracer.Start(ctx, name)

	return &Eotel{
		ctx:          ctx,
		span:         span,
		logger:       l.logger,
		tracer:       tracer,
		meter:        l.meter,
		logCounter:   l.logCounter,
		durationHist: l.durationHist,
		exporter:     l.exporter,
		name:         name,
		start:        time.Now(),
	}
}

func Noop(name string) *Eotel {
	return &Eotel{
		ctx:    context.Background(),
		logger: zap.NewNop(),
		name:   name,
		start:  time.Now(),
	}
}

type Timer interface {
	Stop()
}

type eotelTimer struct {
	name   string
	logger *Eotel
	start  time.Time
}

func (l *Eotel) Start(name string) Timer {
	start := time.Now()
	return &eotelTimer{name: name, logger: l, start: start}
}

func (t *eotelTimer) Stop() {
	duration := time.Since(t.start).Seconds() * 1000
	t.logger.SpanEvent(t.name, attribute.Float64("custom.duration_ms", duration))
}
