package trigger

type RedisConfig struct {
	Stream    string `json:"stream"`
	Group     string `json:"group"`
	BatchSize int    `json:"batch_size"`
}

type PollingConfig struct {
	URL          string            `json:"url"`
	Method       string            `json:"method"`
	IntervalSec  int               `json:"interval_sec"`
	ItemsPath    string            `json:"items_path"`
	CursorPath   string            `json:"cursor_path"`
	CursorParam  string            `json:"cursor_param"`
	AuthHeader   string            `json:"auth_header"`
	Headers      map[string]string `json:"headers,omitempty"`
	AllowPrivate bool              `json:"allow_private"`
}

type OpenAPIConfig struct {
	SyncWaitMS int `json:"sync_wait_ms"`
}
