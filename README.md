# eotel

ชุดเครื่องมือ Observability แบบเบาๆ สำหรับ Go:
- ✅ Logger (Zap)
- ✅ Distributed Tracing & Metrics (OpenTelemetry → OTLP/Collector)
- ✅ Panic guard + Gin middleware
- ✅ Sentry error reporting (เลือกเปิดได้)
- ✅ Loki log shipping (เลือกเปิดได้; มี helper ส่งแบบ async)

> เหมาะสำหรับบริการที่ใช้ **Gin** และต้องการ context-aware logger + span อัตโนมัติ

---

## คุณสมบัติ
- Hexagonal-friendly: ใช้ผ่าน context, แยก concern ชัดเจน
- Lazy span: เริ่ม span อัตโนมัติเมื่อมีการ log ครั้งแรก
- Metrics ติดมากับ log: `log_total` (counter) และ `log_duration_ms` (histogram)
- ส่ง error เข้า Sentry ได้ทันที และรองรับ Loki แบบ async helper

---

## ความต้องการระบบ
- Go 1.24+ (แนะนำ)
- OpenTelemetry Collector (OTLP gRPC)
- (ถ้ามี) Sentry, Loki

---

## ติดตั้ง
```bash
go get github.com/nicedev97/eotel-v2
```

### การตั้งค่าเริ่มต้น (Init)
```go
package main

import (
	"context"
	"log"

	"github.com/nicedev97/eotel-v2"
)

func main() {
	ctx := context.Background()

	shutdown, err := eotel.InitEOTEL(ctx, eotel.Config{
		ServiceName:   "my-service",
		JobName:       "my-job",
		OtelCollector: "otel-collector:4317",

		// เปิด/ปิดความสามารถ
		EnableTracing: true,
		EnableMetrics: true,
		EnableSentry:  true,
		EnableLoki:    true,

		// OTLP: TLS ใน production
		OTLPUseTLS: false, // true เพื่อใช้ TLS

		// ปลายทางส่งออก
		SentryDSN: "<your-sentry-dsn>",
		LokiURL:   "http://loki:3100/loki/api/v1/push",
	})
	if err != nil {
		log.Fatal(err)
	}
	defer shutdown(context.Background())

	// ... start server / jobs
}
```

## ตาราง Methods และฟังก์ชัน

### โครงสร้าง Config
| Field         | Type   | อธิบาย                                       |
|---------------|--------|----------------------------------------------|
| ServiceName   | string | ชื่อบริการ (ใช้กับ OTel/Zap labels)         |
| JobName       | string | ชื่อ job/process (label เสริม)              |
| OtelCollector | string | ที่อยู่ OTLP gRPC (เช่น `otel-collector:4317`)|
| EnableTracing | bool   | เปิด/ปิด tracing                             |
| EnableMetrics | bool   | เปิด/ปิด metrics                             |
| EnableSentry  | bool   | เปิด/ปิด Sentry                              |
| EnableLoki    | bool   | เปิด/ปิด Loki (helper ส่ง log async)         |
| OTLPUseTLS| bool   | ใช้ TLS สำหรับ OTLP (แนะนำ production)|
| SentryDSN     | string | DSN ของ Sentry                                |
| LokiURL       | string | Endpoint Loki `/loki/api/v1/push`            |

### ฟังก์ชันระดับแพ็กเกจ
| Function                                                | Signature                                                                     | อธิบาย                                                                                       |
|---------------------------------------------------------|--------------------------------------------------------------------------------|----------------------------------------------------------------------------------------------|
| InitEOTEL                                               | InitEOTEL(ctx context.Context, cfg Config) (func(context.Context) error, error)| ตั้ง Resource + Tracer/Meter Provider + Sentry; คืน shutdown สำหรับ flush/ปิดทรัพยากร       |
| Middleware                                              | Middleware(name string) gin.HandlerFunc                                        | Gin middleware: เริ่ม span ต่อ request, inject logger, log "request completed"               |
| RecoverPanic                                            | RecoverPanic(c *gin.Context) func()                                            | กัน panic → ตอบ 500 + บันทึก error + set span status                                        |
| Inject                                                  | Inject(ctx context.Context, logger *Eotel) context.Context                     | ใส่ *Eotel ลง context                                                                        |
| FromContext                                             | FromContext(ctx context.Context, name string) *Eotel                           | ดึง *Eotel จาก context (ถ้าไม่มี คืน Noop)                                                  |
| FromGin                                                 | FromGin(c *gin.Context, name string) *Eotel                                    | ดึง logger จาก Gin context                                                                   |
| Safe                                                    | Safe(l *Eotel) *Eotel                                                          | กัน nil; ถ้า l == nil จะคืน Noop("safe")                                                    |
| Noop                                                    | Noop(name string) *Eotel                                                       | logger ว่าง (ไม่ทำอะไร)                                                                      |
| CaptureError                                            | CaptureError(err error, tags map[string]string, extras map[string]interface{}) | ส่ง exception ไป Sentry (ถ้าเปิดใช้)                                                        |
| SendLokiAsync                                           | SendLokiAsync(level, msg, traceID, spanID string)                              | ส่ง log เข้า Loki แบบ asynchronous ผ่าน channel ภายใน                                       |

### ชนิดข้อมูล Eotel และ Methods
| Method / Field        | Signature                                                                    | อธิบาย                                                                    |
|-----------------------|------------------------------------------------------------------------------|---------------------------------------------------------------------------|
| TraceName             | (l *Eotel) TraceName(name string) *Eotel                                     | ตั้งชื่อ span/log ปัจจุบัน                                                |
| Info/Debug/Warn/Error | (l *Eotel) <Level>(msg string)                                               | เขียน log + set attributes + metrics และปิด span (Fatal จะ os.Exit(1))    |
| WithField             | (l *Eotel) WithField(key string, value any) *Eotel                           | เพิ่ม Zap field + OTel attribute                                          |
| WithFields            | (l *Eotel) WithFields(m map[string]any) *Eotel                               | เพิ่มหลายฟิลด์รวดเดียว                                                    |
| WithError             | (l *Eotel) WithError(err error) *Eotel                                       | ผูก error กับ logger + attribute "error" (และเรียก exporter ถ้ามี)        |
| Ctx                   | (l *Eotel) Ctx() context.Context                                             | คืน context ปัจจุบัน                                                      |
| Span                  | (l *Eotel) Span() trace.Span                                                 | คืน span ปัจจุบัน (อาจเป็น nil)                                           |
| WithTracer            | (l *Eotel) WithTracer(name string, fn func(ctx context.Context) error) error | สร้าง span เฉพาะกิจแล้วรัน fn ภายใน                                       |
| SpanEvent             | (l *Eotel) SpanEvent(name string, attrs ...attribute.KeyValue)               | เพิ่ม event ลงใน span                                                     |
| SetSpanAttr           | (l *Eotel) SetSpanAttr(key string, value any)                                | ตั้ง attribute ให้ span                                                   |
| SetSpanError          | (l *Eotel) SetSpanError(err error)                                           | Record error ลง span                                                      |
| Child                 | (l *Eotel) Child(name string) *Eotel                                         | สร้าง child span + child logger (สืบทอด tracer/meter/exporter จาก parent) |
| Start / Timer.Stop    | (l *Eotel) Start(name string) Timer  /  (t *eotelTimer) Stop()               | ตัวจับเวลาแบบง่าย: บันทึก event `custom.duration_ms` เมื่อ Stop()         |
| NewWithExporter       | NewWithExporter(ctx context.Context, name string, exp Exporter) *Eotel       | สร้าง logger ที่ใช้ exporter กำหนดเอง                                     |
| SetExporter           | (l *Eotel) SetExporter(exp Exporter) *Eotel                                  | เปลี่ยน exporter runtime                                                  |

## การใช้งานเพิ่มเติม
```go
// main.go
package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/nicedev97/eotel-v2"
	"github.com/gin-gonic/gin"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
)

func main() {
	// ---------- 1) InitEOTEL: ตั้งค่า Tracing/Metrics/Sentry/Loki ----------
	ctx := context.Background()

	shutdown, err := eotel.InitEOTEL(ctx, eotel.Config{
		ServiceName:   "demo-eotel",
		JobName:       "api",
		OtelCollector: "otel-collector:4317",

		EnableTracing: true,
		EnableMetrics: true,
		EnableSentry:  true,
		EnableLoki:    true,

		SentryDSN: "<your-sentry-dsn>",
		LokiURL:   "http://loki:3100/loki/api/v1/push",
	})
	if err != nil {
		log.Fatal(err)
	}
	defer shutdown(context.Background())

	// ---------- 2) สร้าง Gin + Middleware: เริ่ม root-span ต่อ request + inject logger ----------
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(eotel.Middleware("gin")) // <- ใช้ Middleware(name)

	// Health/simple
	r.GET("/health", func(c *gin.Context) {
		eotel.FromGin(c, "Health").Info("ok")
		c.String(http.StatusOK, "ok")
	})

	// ---------- 3) ตัวอย่างใช้งานครบๆ ใน handler ----------
	r.GET("/demo", func(c *gin.Context) {
		// ดึง logger จาก Gin context แล้วตั้งชื่อ span
		logx := eotel.FromGin(c, "Demo").TraceName("DemoHandler").
			WithField("path", c.FullPath()).
			WithFields(map[string]any{
				"method": c.Request.Method,
				"ip":     c.ClientIP(),
			})

		// เพิ่ม attribute ให้ span และเพิ่ม event
		logx.SetSpanAttr("demo.flag", true)
		logx.SpanEvent("step.start", attribute.String("k", "v"))

		// Timer ง่ายๆ
		t := logx.Start("block-A")
		time.Sleep(15 * time.Millisecond)
		t.Stop() // -> span event: custom.duration_ms

		// ใช้ Child span
		child := logx.Child("query-db").WithField("retry", 0)
		time.Sleep(5 * time.Millisecond)
		child.Debug("query finished")

		// เข้าถึง span ปัจจุบัน (ถ้าต้องการ)
		if sp := logx.Span(); sp != nil {
			sc := sp.SpanContext()
			logx.WithField("trace_id", sc.TraceID().String())
		}

		logx.Info("demo done")
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})

	// ---------- 4) WithTracer + SetSpanError + WithError + CaptureError ----------
	r.GET("/error", func(c *gin.Context) {
		logx := eotel.FromGin(c, "ErrorFlow").TraceName("ErrorFlow")
		_ = logx.WithTracer("Compute", func(ctx context.Context) error {
			// สร้าง error และแนบกับ span และ logger
			err := errors.New("compute failed")
			logx.SetSpanError(err)
			logx.WithError(err).Error("compute failed with error")

			// ส่งให้ Sentry เพิ่มเติม (optional)
			eotel.CaptureError(err, map[string]string{"endpoint": "error"}, map[string]any{"feature": "demo"})

			return err
		})
		c.JSON(http.StatusInternalServerError, gin.H{"error": "compute failed"})
	})

	// ---------- 5) Loki: ส่ง log แบบ async ตาม trace/span ปัจจุบัน ----------
	r.GET("/loki", func(c *gin.Context) {
		logx := eotel.FromGin(c, "LokiDemo").TraceName("LokiDemo")
		sc := trace.SpanFromContext(logx.Ctx()).SpanContext()
		eotel.SendLokiAsync("info", "hello from loki", sc.TraceID().String(), sc.SpanID().String())
		logx.Info("sent to loki")
		c.String(http.StatusOK, "ok")
	})

	// ---------- 6) RecoverPanic แบบ manual (นอก middleware) ----------
	r.GET("/panic", func(c *gin.Context) {
		defer eotel.RecoverPanic(c)() // <- ใช้ RecoverPanic(c)
		panic("boom!")               // จะถูกจับ → 500 + บันทึก error + set span status
	})

	// ---------- 7) Inject/FromContext: ใช้นอก Gin หรืองานลึก ----------
	r.GET("/inject", func(c *gin.Context) {
		// สร้าง logger เองแล้ว Inject ลง context เพื่อส่งต่อไปฟังก์ชันอื่น
		manual := eotel.New(c.Request.Context(), "ManualLogger").TraceName("Manual").
			WithField("who", "manual-inject")
		ctx2 := eotel.Inject(c.Request.Context(), manual) // <- Inject(ctx, logger)

		// Call service layer ที่ภายในจะ FromContext(...) เอง
		if err := serviceWork(ctx2); err != nil {
			manual.WithError(err).Error("serviceWork failed")
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		manual.Info("serviceWork success")
		c.String(http.StatusOK, "ok")
	})

	// ---------- 8) Safe/Noop: กัน nil logger ----------
	r.GET("/safe", func(c *gin.Context) {
		var nilLogger *eotel.Eotel
		eotel.Safe(nilLogger).Info("this will not panic (noop)") // <- Safe(nil) ใช้ได้
		eotel.Noop("noop").Debug("also noop debug")              // <- Noop(...)
		c.String(http.StatusOK, "ok")
	})

	// ---------- 9) Warn/Debug ใช้งานทั่วไป ----------
	r.GET("/levels", func(c *gin.Context) {
		l := eotel.FromGin(c, "Levels")
		l.Debug("this is debug")
		l.Warn("this is warn")
		// l.Fatal("fatal will os.Exit(1)") // ตัวอย่าง: ระวัง! จะจบโปรเซส
		l.Info("levels done")
		c.String(http.StatusOK, "ok")
	})

	// ---------- 10) งาน background/cron ที่อยู่นอก HTTP ----------
	go func() {
		tick := time.NewTicker(5 * time.Second)
		defer tick.Stop()
		for range tick.C {
			cronJob(context.Background())
		}
	}()

	_ = r.Run(":8080")
}

// --------- ตัวอย่าง Service Layer: ใช้ FromContext/Safe/Child/WithTracer/SpanEvent ----------
func serviceWork(ctx context.Context) error {
	logx := eotel.Safe(eotel.FromContext(ctx, "ServiceWork")).TraceName("ServiceWork")

	// ทำงานย่อยด้วย Child span
	child := logx.Child("prepare")
	child.SpanEvent("prepare.start")
	time.Sleep(2 * time.Millisecond)
	child.SpanEvent("prepare.done")
	child.Info("prepared")

	// ครอบด้วย WithTracer
	return logx.WithTracer("Exec", func(ctx context.Context) error {
		l := eotel.FromContext(ctx, "Exec").WithFields(map[string]any{
			"attempt": 1,
			"mode":    "fast",
		})
		timer := l.Start("io")
		time.Sleep(3 * time.Millisecond)
		timer.Stop()

		// จำลองเงื่อนไขเตือน
		l.Warn("io slow a bit")

		// ใส่ event + attribute เพิ่ม
		l.SpanEvent("exec.step", attribute.Int("n", 1))
		l.SetSpanAttr("exec.user", "demo")

		l.Info("exec ok")
		return nil
	})
}

// --------- งานเบื้องหลัง (ไม่มี Gin): ใช้ New + Inject + FromContext ----------
func cronJob(parent context.Context) {
	root := eotel.New(parent, "Cron").TraceName("CronTick")
	defer root.Info("cron tick finished")

	// Inject แล้วส่งไปฟังก์ชันย่อย
	ctx := eotel.Inject(parent, root)
	doSubTask(ctx)
}

func doSubTask(ctx context.Context) {
	l := eotel.FromContext(ctx, "SubTask")
	l.Debug("sub-task started")
	time.Sleep(1 * time.Millisecond)
	l.Info("sub-task done")
	fmt.Print("") // no-op เพื่อกัน unused import warning ในบางกรณี
}
```

### การใช้งาน Exporter
