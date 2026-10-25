package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

type mockModel struct {
	server   *http.Server
	listener net.Listener
	mu       sync.Mutex
	requests int64
	lastBody string
}

func startMockModel(addr string) (*mockModel, error) {
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}
	m := &mockModel{listener: listener}
	mux := http.NewServeMux()
	mux.HandleFunc("/", m.handle)
	m.server = &http.Server{Handler: mux}
	go func() { _ = m.server.Serve(listener) }()
	return m, nil
}

func (m *mockModel) Close() {
	if m.server != nil {
		_ = m.server.Close()
	}
}

func (m *mockModel) handle(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("X-QA-Fail") == "1" {
		m.mu.Lock()
		m.requests++
		m.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":{"message":"QA100 provider header forced failure"}}`))
		return
	}
	raw, _ := io.ReadAll(io.LimitReader(r.Body, 4<<20))
	body := string(raw)
	m.mu.Lock()
	m.requests++
	m.lastBody = body
	m.mu.Unlock()

	if strings.Contains(body, "QA_MODEL_FAIL") {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":{"message":"QA100 forced model failure"}}`))
		return
	}
	content := mockVerdictJSON(body)
	resp := map[string]any{
		"id": "qa100-mock", "object": "chat.completion", "created": time.Now().Unix(),
		"model": "qa-mock",
		"choices": []any{map[string]any{
			"index":         0,
			"message":       map[string]any{"role": "assistant", "content": content},
			"finish_reason": "stop",
		}},
		"usage": map[string]any{"prompt_tokens": 120, "completion_tokens": 48, "total_tokens": 168},
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

func mockVerdictJSON(prompt string) string {
	if strings.Contains(prompt, `Return exactly {"ok":true}`) {
		return `{"ok":true}`
	}
	verdict := map[string]any{
		"result": "PASS", "risk_level": "low", "score": 0.10,
		"labels": []string{"QA100"}, "summary": "QA100 mock PASS",
		"reasons": []string{"automated mock"},
	}
	switch {
	case strings.Contains(prompt, "QA_CLASSIFY_REJECT"):
		verdict["result"] = "REJECT"
		verdict["risk_level"] = "high"
		verdict["score"] = 0.93
		verdict["category"] = "risk"
		verdict["category_name"] = "风险"
		verdict["category_confidence"] = 0.99
		verdict["summary"] = "QA100 mock classify REJECT"
	case strings.Contains(prompt, "QA_CLASSIFY_REVIEW"):
		verdict["result"] = "REVIEW"
		verdict["risk_level"] = "medium"
		verdict["score"] = 0.55
		verdict["category"] = "manual"
		verdict["category_name"] = "待定"
		verdict["category_confidence"] = 0.86
		verdict["summary"] = "QA100 mock classify REVIEW"
	case strings.Contains(prompt, "QA_CLASSIFY_PASS"):
		verdict["category"] = "safe"
		verdict["category_name"] = "安全"
		verdict["category_confidence"] = 0.99
		verdict["summary"] = "QA100 mock classify PASS"
	case strings.Contains(prompt, "QA_LOW_CONF"):
		verdict["category"] = "safe"
		verdict["category_name"] = "安全"
		verdict["category_confidence"] = 0.41
		verdict["score"] = 0.52
		verdict["summary"] = "QA100 mock low confidence"
	case strings.Contains(prompt, "QA_UNKNOWN"):
		verdict["category"] = "unmapped_type"
		verdict["category_name"] = "未知"
		verdict["category_confidence"] = 0.90
		verdict["summary"] = "QA100 mock unknown category"
	case strings.Contains(prompt, "QA_REJECT"):
		verdict["result"] = "REJECT"
		verdict["risk_level"] = "high"
		verdict["score"] = 0.95
		verdict["summary"] = "QA100 mock REJECT"
	case strings.Contains(prompt, "QA_REVIEW"):
		verdict["result"] = "REVIEW"
		verdict["risk_level"] = "medium"
		verdict["score"] = 0.50
		verdict["summary"] = "QA100 mock REVIEW"
	}
	raw, _ := json.Marshal(verdict)
	return string(raw)
}

type actionRecord struct {
	Kind     string      `json:"kind"`
	TaskNo   string      `json:"task_no"`
	Result   string      `json:"result"`
	Audit    string      `json:"audit"`
	Path     string      `json:"path"`
	Body     string      `json:"body"`
	Headers  http.Header `json:"-"`
	Received time.Time   `json:"received_at"`
}

type recorder struct {
	mu      sync.Mutex
	records map[string][]*actionRecord
}

func newRecorder() *recorder {
	return &recorder{records: map[string][]*actionRecord{}}
}

func (r *recorder) add(kind string, rec *actionRecord) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.records[kind] = append(r.records[kind], rec)
}

func (r *recorder) forTask(kind, taskNo string) []*actionRecord {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []*actionRecord
	for _, rec := range r.records[kind] {
		if rec.TaskNo == taskNo {
			out = append(out, rec)
		}
	}
	return out
}

func (r *recorder) wait(kind, taskNo string, want int, timeout time.Duration) ([]*actionRecord, error) {
	deadline := time.Now().Add(timeout)
	for {
		items := r.forTask(kind, taskNo)
		if len(items) >= want {
			return items, nil
		}
		if time.Now().After(deadline) {
			return items, fmt.Errorf("timeout waiting for %s task=%s want=%d got=%d", kind, taskNo, want, len(items))
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func (r *recorder) snapshot() map[string]int {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := map[string]int{}
	for kind, items := range r.records {
		out[kind] = len(items)
	}
	return out
}

type actionServer struct {
	server        *http.Server
	listener      net.Listener
	rec           *recorder
	flakyMu       sync.Mutex
	flakyAttempts map[string]int
}

func startActionServer(addr string) (*actionServer, error) {
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}
	as := &actionServer{listener: listener, rec: newRecorder(), flakyAttempts: map[string]int{}}
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("/action/http", func(w http.ResponseWriter, r *http.Request) {
		as.handleHTTP(w, r, "http-action")
	})
	mux.HandleFunc("/action/js", func(w http.ResponseWriter, r *http.Request) {
		as.handleHTTP(w, r, "js-action")
	})
	mux.HandleFunc("/callback/http", func(w http.ResponseWriter, r *http.Request) {
		as.handleHTTP(w, r, "http-callback")
	})
	mux.HandleFunc("/callback/flaky", as.handleFlakyCallback)
	mux.HandleFunc("/ws-action", func(w http.ResponseWriter, r *http.Request) {
		as.handleWS(w, r, "ws-action")
	})
	mux.HandleFunc("/ws-callback", func(w http.ResponseWriter, r *http.Request) {
		as.handleWS(w, r, "ws-callback")
	})
	as.server = &http.Server{Handler: mux}
	go func() { _ = as.server.Serve(listener) }()
	return as, nil
}

func (a *actionServer) Close() {
	if a.server != nil {
		_ = a.server.Close()
	}
}

func (a *actionServer) handleHTTP(w http.ResponseWriter, r *http.Request, kind string) {
	raw, _ := io.ReadAll(io.LimitReader(r.Body, 2<<20))
	taskNo, result, audit := fieldsFromJSON(raw)
	a.rec.add(kind, &actionRecord{
		Kind: kind, TaskNo: taskNo, Result: result, Audit: audit, Path: r.URL.Path,
		Body: string(raw), Headers: r.Header.Clone(), Received: time.Now(),
	})
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"ok":true}`))
}

func (a *actionServer) handleFlakyCallback(w http.ResponseWriter, r *http.Request) {
	raw, _ := io.ReadAll(io.LimitReader(r.Body, 2<<20))
	taskNo, result, audit := fieldsFromJSON(raw)
	a.rec.add("http-callback", &actionRecord{
		Kind: "http-callback", TaskNo: taskNo, Result: result, Audit: audit, Path: r.URL.Path,
		Body: string(raw), Headers: r.Header.Clone(), Received: time.Now(),
	})
	a.flakyMu.Lock()
	a.flakyAttempts[taskNo]++
	attempt := a.flakyAttempts[taskNo]
	a.flakyMu.Unlock()
	if attempt < 3 {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"ok":false,"attempt":` + fmt.Sprint(attempt) + `}`))
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"ok":true}`))
}

func (a *actionServer) handleWS(w http.ResponseWriter, r *http.Request, kind string) {
	upgrader := websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()
	_ = conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	for {
		_, msg, err := conn.ReadMessage()
		if err != nil {
			return
		}
		taskNo, result, audit := fieldsFromJSON(msg)
		a.rec.add(kind, &actionRecord{
			Kind: kind, TaskNo: taskNo, Result: result, Audit: audit, Path: r.URL.Path,
			Body: string(msg), Headers: r.Header.Clone(), Received: time.Now(),
		})
	}
}

func fieldsFromJSON(raw []byte) (taskNo, result, audit string) {
	var value map[string]any
	if err := json.Unmarshal(raw, &value); err != nil {
		return "", "", ""
	}
	for _, key := range []string{"task", "task_no", "biz_key"} {
		if v, ok := value[key]; ok && fmt.Sprint(v) != "" {
			taskNo = fmt.Sprint(v)
			break
		}
	}
	for _, key := range []string{"result", "conclusion"} {
		if v, ok := value[key]; ok && fmt.Sprint(v) != "" {
			result = strings.ToUpper(fmt.Sprint(v))
			break
		}
	}
	for _, key := range []string{"audit", "audit_code"} {
		if v, ok := value[key]; ok && fmt.Sprint(v) != "" {
			audit = fmt.Sprint(v)
			break
		}
	}
	return taskNo, result, audit
}
