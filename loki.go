package eotel

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"
)

type LokiEntry struct {
	Labels  map[string]string
	Message string
}

func SendLokiAsync(level string, msg string, traceID string, spanID string) {
	if !globalCfg.EnableLoki {
		return
	}
	labels := map[string]string{
		"level":    level,
		"trace_id": traceID,
		"span_id":  spanID,
		"service":  globalCfg.ServiceName,
		"job":      globalCfg.JobName,
	}

	logChan <- LokiEntry{Labels: labels, Message: msg}
}

var logChan = make(chan LokiEntry, 100)

func init() {
	go func() {
		for entry := range logChan {
			b, _ := json.Marshal(entry)
			log.Println(string(b))

			_ = sendLoki(entry)
		}
	}()
}

func sendLoki(entry LokiEntry) error {
	if !globalCfg.EnableLoki {
		return nil
	}
	ts := time.Now().UnixNano()
	body := map[string]interface{}{
		"streams": []map[string]interface{}{
			{
				"stream": entry.Labels,
				"values": [][2]string{{fmt.Sprintf("%d", ts), entry.Message}},
			},
		},
	}
	data, _ := json.Marshal(body)
	resp, err := http.Post(globalCfg.LokiURL, "application/json", bytes.NewBuffer(data))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("loki response: %s", resp.Status)
	}
	return nil
}
