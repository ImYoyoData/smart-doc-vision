package metrics

import (
	"fmt"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
)

type key struct {
	method string
	route  string
	status int
}

var (
	startedAt = time.Now()
	mu        sync.Mutex
	requests  = map[key]uint64{}
)

// ObserveRequest records one completed HTTP request. route should be the
// registered Gin route (not the raw URL) to keep label cardinality bounded.
func ObserveRequest(method, route string, status int) {
	if route == "" {
		route = "unmatched"
	}
	k := key{method: method, route: route, status: status}
	mu.Lock()
	requests[k]++
	mu.Unlock()
}

// Render writes process and HTTP metrics in Prometheus text exposition format.
func Render(extra ...string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# HELP smartdoc_build_info Build information.\n")
	fmt.Fprintf(&b, "# TYPE smartdoc_build_info gauge\n")
	mu.Lock()
	snapshot := make([]key, 0, len(requests))
	counts := make(map[key]uint64, len(requests))
	for k, v := range requests {
		snapshot = append(snapshot, k)
		counts[k] = v
	}
	mu.Unlock()
	sort.Slice(snapshot, func(i, j int) bool {
		a, c := snapshot[i], snapshot[j]
		if a.route != c.route {
			return a.route < c.route
		}
		if a.method != c.method {
			return a.method < c.method
		}
		return a.status < c.status
	})
	fmt.Fprintf(&b, "# HELP smartdoc_uptime_seconds Process uptime in seconds.\n")
	fmt.Fprintf(&b, "# TYPE smartdoc_uptime_seconds gauge\n")
	fmt.Fprintf(&b, "smartdoc_uptime_seconds %.0f\n", time.Since(startedAt).Seconds())
	fmt.Fprintf(&b, "# HELP smartdoc_goroutines Current goroutine count.\n")
	fmt.Fprintf(&b, "# TYPE smartdoc_goroutines gauge\n")
	fmt.Fprintf(&b, "smartdoc_goroutines %d\n", runtime.NumGoroutine())
	fmt.Fprintf(&b, "# HELP smartdoc_http_requests_total Completed HTTP requests by route and status.\n")
	fmt.Fprintf(&b, "# TYPE smartdoc_http_requests_total counter\n")
	for _, k := range snapshot {
		fmt.Fprintf(&b, "smartdoc_http_requests_total{method=%q,route=%q,status=\"%d\"} %d\n",
			escape(k.method), escape(k.route), k.status, counts[k])
	}
	for _, line := range extra {
		line = strings.TrimRight(line, "\n")
		if line != "" {
			b.WriteString(line)
			b.WriteByte('\n')
		}
	}
	return b.String()
}

func escape(v string) string {
	v = strings.ReplaceAll(v, `\`, `\`)
	return strings.ReplaceAll(v, `"`, `\"`)
}
