// Command qax is an exploratory QA driver: login with captcha bypass (reads
// the answer straight from Redis like qa100), arbitrary admin API calls, and
// direct MySQL/Redis queries for verification. Not for production use.
package main

import (
	"bufio"
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	_ "github.com/go-sql-driver/mysql"
	"github.com/redis/go-redis/v9"
)

type env struct {
	root    string
	baseURL string
	token   string
	redis   *redis.Client
	db      *sql.DB
}

func main() {
	ctx := context.Background()
	reqCmd := flag.NewFlagSet("req", flag.ExitOnError)
	sqlCmd := flag.NewFlagSet("sql", flag.ExitOnError)
	redisCmd := flag.NewFlagSet("redis", flag.ExitOnError)

	if len(os.Args) < 2 {
		usage()
	}
	root := repoRoot()
	te := loadEnv(filepath.Join(root, ".env"))
	get := func(key, fallback string) string {
		if v := os.Getenv(key); v != "" {
			return v
		}
		if v := te[key]; v != "" {
			return v
		}
		return fallback
	}
	base := get("QAX_BASE", "http://127.0.0.1:8080")
	rdb := redis.NewClient(&redis.Options{
		Addr:     get("REDIS_ADDR", "127.0.0.1:6379"),
		Password: get("REDIS_PASSWORD", ""),
		DB:       atoi(get("REDIS_DB", "3")),
	})
	e := &env{root: root, baseURL: base, redis: rdb}
	tokenPath := filepath.Join(root, "tmp", "qax-token.json")

	var err error
	switch os.Args[1] {
	case "login":
		user := get("ADMIN_USERNAME", "admin")
		pass := get("ADMIN_PASSWORD", "")
		if pass == "" {
			fatal("ADMIN_PASSWORD required")
		}
		e.token, err = doLogin(ctx, base, rdb, user, pass)
		fatalIf(err, "login")
		_ = os.MkdirAll(filepath.Dir(tokenPath), 0o755)
		_ = os.WriteFile(tokenPath, []byte(e.token), 0o600)
		fmt.Println("TOKEN SAVED", tokenPath)
	case "req":
		reqCmd.Parse(os.Args[2:])
		args := reqCmd.Args()
		if len(args) < 2 {
			fatal("usage: qax req METHOD /path [jsonBody]")
		}
		b, err := os.ReadFile(tokenPath)
		fatalIf(err, "read token (run `qax login` first)")
		e.token = strings.TrimSpace(string(b))
		var body io.Reader
		if len(args) >= 3 {
			body = strings.NewReader(args[2])
		}
		req, err := http.NewRequestWithContext(ctx, args[0], base+args[1], body)
		fatalIf(err, "request")
		if body != nil {
			req.Header.Set("Content-Type", "application/json")
		}
		req.Header.Set("Authorization", "Bearer "+e.token)
		resp, err := http.DefaultClient.Do(req)
		fatalIf(err, "do request")
		defer resp.Body.Close()
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
		fmt.Printf("HTTP %d\n%s\n", resp.StatusCode, string(raw))
	case "sql":
		sqlCmd.Parse(os.Args[2:])
		query := strings.Join(sqlCmd.Args(), " ")
		if query == "" {
			fatal("usage: qax sql \"SELECT ...\"")
		}
		dsn := get("MYSQL_DSN", "")
		if dsn == "" {
			fatal("MYSQL_DSN required")
		}
		db, err := sql.Open("mysql", dsn)
		fatalIf(err, "open mysql")
		defer db.Close()
		rows, err := db.Query(query)
		fatalIf(err, "query")
		defer rows.Close()
		cols, _ := rows.Columns()
		fmt.Println(strings.Join(cols, "\t"))
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		for rows.Next() {
			if err := rows.Scan(ptrs...); err != nil {
				fatal("scan: %v", err)
			}
			out := make([]string, len(cols))
			for i, v := range vals {
				out[i] = fmt.Sprintf("%v", v)
				if v == nil {
					out[i] = "NULL"
				}
			}
			fmt.Println(strings.Join(out, "\t"))
		}
	case "redis":
		redisCmd.Parse(os.Args[2:])
		args := redisCmd.Args()
		if len(args) < 2 {
			fatal("usage: qax redis get|keys|del ...")
		}
		switch args[0] {
		case "get":
			v, err := rdb.Get(ctx, args[1]).Result()
			fatalIf(err, "redis get")
			fmt.Println(v)
		case "keys":
			keys, err := rdb.Keys(ctx, args[1]).Result()
			fatalIf(err, "redis keys")
			for _, k := range keys {
				fmt.Println(k)
			}
		case "del":
			n, err := rdb.Del(ctx, args[1]).Result()
			fatalIf(err, "redis del")
			fmt.Println("deleted", n)
		default:
			fatal("unknown redis subcommand %q", args[0])
		}
	default:
		usage()
	}
	_ = e
}

func usage() {
	fmt.Fprintln(os.Stderr, "usage: qax login | req METHOD /path [body] | sql \"query\" | redis get|keys|del ...")
	os.Exit(1)
}

func doLogin(ctx context.Context, base string, rdb *redis.Client, user, pass string) (string, error) {
	resp, err := http.Get(base + "/api/admin/v1/auth/captcha")
	if err != nil {
		return "", fmt.Errorf("captcha: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	var ch struct {
		Code any             `json:"code"`
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(raw, &ch); err != nil {
		return "", err
	}
	var data struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(ch.Data, &data); err != nil {
		return "", err
	}
	answer, err := rdb.Get(ctx, "sdv:auth:captcha:"+data.ID).Result()
	if err != nil {
		return "", fmt.Errorf("captcha answer from redis: %w", err)
	}
	var ans struct {
		X int `json:"x"`
	}
	if err := json.Unmarshal([]byte(answer), &ans); err != nil {
		return "", err
	}
	payload := fmt.Sprintf(`{"username":%q,"password":%q,"captcha_id":%q,"captcha_x":%d}`, user, pass, data.ID, ans.X)
	req, err := http.NewRequest(http.MethodPost, base+"/api/admin/v1/auth/login", strings.NewReader(payload))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	resp2, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("login: %w", err)
	}
	defer resp2.Body.Close()
	raw2, _ := io.ReadAll(resp2.Body)
	var out struct {
		Code    any    `json:"code"`
		Message string `json:"message"`
		Data    struct {
			AccessToken string `json:"access_token"`
		} `json:"data"`
	}
	if err := json.Unmarshal(raw2, &out); err != nil {
		return "", err
	}
	if out.Data.AccessToken == "" {
		return "", fmt.Errorf("login failed HTTP %d: %s %s", resp2.StatusCode, out.Message, string(raw2))
	}
	return out.Data.AccessToken, nil
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
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			out[parts[0]] = strings.Trim(parts[1], `"'`)
		}
	}
	return out
}

func repoRoot() string {
	dir, _ := os.Getwd()
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

func atoi(s string) int {
	var out int
	fmt.Sscanf(s, "%d", &out)
	return out
}

func fatalIf(err error, what string) {
	if err != nil {
		fatal("%s: %v", what, err)
	}
}

func fatal(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "qax: "+format+"\n", args...)
	os.Exit(1)
}
