package main

import (
	"bufio"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

type state struct {
	RunID       string    `json:"run_id"`
	ProjectID   uint64    `json:"project_id"`
	ProjectCode string    `json:"project_code"`
	ProviderID  uint64    `json:"provider_id"`
	RedisConnID uint64    `json:"redis_conn_id"`
	CreatedAt   time.Time `json:"created_at"`
}

type testEnv struct {
	root       string
	baseURL    string
	mockAddr   string
	actionAddr string
	statePath  string
	reportPath string
	username   string
	password   string
	mysqlDSN   string
	redisAddr  string
	redisPass  string
	redisDB    int
	state      state
}

func main() {
	phase := flag.String("phase", "run", "setup | run | cleanup")
	start := flag.Int("start", 1, "first round")
	end := flag.Int("end", 10, "last round")
	base := flag.String("base", "http://127.0.0.1:8080", "backend base URL")
	mockAddr := flag.String("mock", "127.0.0.1:18080", "mock model listen address")
	actionAddr := flag.String("actions", "127.0.0.1:18081", "HTTP/WS action receiver listen address")
	statePath := flag.String("state", "", "state file path")
	reportPath := flag.String("report", "", "JSONL report path")
	flag.Parse()

	root := repoRoot()
	env := loadEnv(filepath.Join(root, ".env"))
	get := func(key, fallback string) string {
		if v := os.Getenv(key); v != "" {
			return v
		}
		if v := env[key]; v != "" {
			return v
		}
		return fallback
	}
	if *statePath == "" {
		*statePath = filepath.Join(root, "tmp", "qa100-state.json")
	}
	if *reportPath == "" {
		*reportPath = filepath.Join(root, "tmp", "qa100-report.jsonl")
	}
	redisDB := 3
	if raw := get("REDIS_DB", "3"); raw != "" {
		fmt.Sscanf(raw, "%d", &redisDB)
	}
	te := &testEnv{
		root:       root,
		baseURL:    strings.TrimRight(*base, "/"),
		mockAddr:   *mockAddr,
		actionAddr: *actionAddr,
		statePath:  *statePath,
		reportPath: *reportPath,
		username:   get("ADMIN_USERNAME", "admin"),
		password:   get("ADMIN_PASSWORD", ""),
		mysqlDSN:   get("MYSQL_DSN", ""),
		redisAddr:  get("REDIS_ADDR", "127.0.0.1:6379"),
		redisPass:  get("REDIS_PASSWORD", ""),
		redisDB:    redisDB,
	}
	if te.password == "" {
		fatal("ADMIN_PASSWORD is required")
	}
	if te.mysqlDSN == "" {
		fatal("MYSQL_DSN is required")
	}
	if err := os.MkdirAll(filepath.Dir(te.statePath), 0o755); err != nil {
		fatal("create state dir: %v", err)
	}

	ctx := context.Background()
	appRedis := redis.NewClient(&redis.Options{Addr: te.redisAddr, Password: te.redisPass, DB: te.redisDB})
	defer appRedis.Close()
	client := newAPIClient(te.baseURL, appRedis)

	switch *phase {
	case "setup":
		mock, err := startMockModel(te.mockAddr)
		if err != nil {
			fatal("start mock model: %v", err)
		}
		defer mock.Close()
		if err := login(ctx, client, te); err != nil {
			fatal("login: %v", err)
		}
		st, err := setupTestProject(ctx, client, te)
		if err != nil {
			fatal("setup: %v", err)
		}
		if err := saveState(te.statePath, st); err != nil {
			fatal("save state: %v", err)
		}
		fmt.Printf("SETUP OK project=%d code=%s provider=%d redis=%d\n", st.ProjectID, st.ProjectCode, st.ProviderID, st.RedisConnID)
	case "run":
		if *start < 1 || *end < *start || *end > 1000 {
			fatal("invalid round range %d..%d", *start, *end)
		}
		st, err := loadState(te.statePath)
		if err != nil {
			fatal("load state: %v", err)
		}
		te.state = st
		if err := login(ctx, client, te); err != nil {
			fatal("login: %v", err)
		}
		if err := verifyTestProject(ctx, client, st); err != nil {
			fatal("verify test project: %v", err)
		}
		mock, err := startMockModel(te.mockAddr)
		if err != nil {
			fatal("start mock model: %v", err)
		}
		defer mock.Close()
		actions, err := startActionServer(te.actionAddr)
		if err != nil {
			fatal("start action server: %v", err)
		}
		defer actions.Close()
		db, err := openTestDB(te.mysqlDSN)
		if err != nil {
			fatal("open mysql: %v", err)
		}
		defer db.Close()
		actionRedis := redis.NewClient(&redis.Options{Addr: te.redisAddr, Password: te.redisPass, DB: 14})
		defer actionRedis.Close()
		runner := &runner{
			env: te, client: client, state: st,
			mock: mock, actions: actions, appRedis: appRedis,
			actionRedis: actionRedis, db: db,
			roundStart: *start,
		}
		if err := runner.runRounds(ctx, *start, *end); err != nil {
			fatal("run rounds: %v", err)
		}
	case "deep":
		st, err := loadState(te.statePath)
		if err != nil {
			fatal("load state: %v", err)
		}
		te.state = st
		if err := login(ctx, client, te); err != nil {
			fatal("login: %v", err)
		}
		if err := verifyTestProject(ctx, client, st); err != nil {
			fatal("verify test project: %v", err)
		}
		mock, err := startMockModel(te.mockAddr)
		if err != nil {
			fatal("start mock model: %v", err)
		}
		defer mock.Close()
		actions, err := startActionServer(te.actionAddr)
		if err != nil {
			fatal("start action server: %v", err)
		}
		defer actions.Close()
		db, err := openTestDB(te.mysqlDSN)
		if err != nil {
			fatal("open mysql: %v", err)
		}
		defer db.Close()
		actionRedis := redis.NewClient(&redis.Options{Addr: te.redisAddr, Password: te.redisPass, DB: 14})
		defer actionRedis.Close()
		runner := &runner{
			env: te, client: client, state: st,
			mock: mock, actions: actions, appRedis: appRedis,
			actionRedis: actionRedis, db: db,
		}
		if err := runner.runDeepChecks(ctx); err != nil {
			fatal("deep checks: %v", err)
		}
	case "cleanup":
		st, err := loadState(te.statePath)
		if err != nil {
			fatal("load state: %v", err)
		}
		te.state = st
		if err := cleanupTestProject(ctx, te, client); err != nil {
			fatal("cleanup: %v", err)
		}
		_ = os.Remove(te.statePath)
		fmt.Printf("CLEANUP OK project=%d\n", st.ProjectID)
	default:
		fatal("unknown phase %q", *phase)
	}
}

func repoRoot() string {
	dir, err := os.Getwd()
	if err != nil {
		fatal("getwd: %v", err)
	}
	for {
		if fileExists(filepath.Join(dir, ".env")) && fileExists(filepath.Join(dir, "backend", "go.mod")) {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			fatal("repository root not found")
		}
		dir = parent
	}
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func loadEnv(path string) map[string]string {
	out := map[string]string{}
	file, err := os.Open(path)
	if err != nil {
		return out
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		if strings.HasPrefix(line, "export ") {
			line = strings.TrimSpace(strings.TrimPrefix(line, "export "))
		}
		key, value, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		value = strings.TrimSpace(value)
		value = strings.Trim(value, `"'`)
		out[strings.TrimSpace(key)] = value
	}
	return out
}

func saveState(path string, st state) error {
	raw, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, raw, 0o644)
}

func loadState(path string) (state, error) {
	var st state
	raw, err := os.ReadFile(path)
	if err != nil {
		return st, err
	}
	err = json.Unmarshal(raw, &st)
	return st, err
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "FATAL: "+format+"\n", args...)
	os.Exit(1)
}
