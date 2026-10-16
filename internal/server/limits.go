package server

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

func (s *HTTPServer) rateLimit(next http.Handler) http.Handler {
	type bucket struct {
		start time.Time
		count int
	}
	buckets := map[string]bucket{}
	var mu sync.Mutex
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.cfg.IdentitySecret != "" && r.Method == "POST" && strings.Contains(r.URL.Path, "/auth/") {
			// The listener is loopback-only. Nginx overwrites, never appends, this header.
			ip, _, _ := net.SplitHostPort(r.RemoteAddr)
			if s.cfg.TrustProxy {
				if parsed := net.ParseIP(r.Header.Get("X-Real-IP")); parsed != nil {
					ip = parsed.String()
				}
			}
			now := time.Now()
			mu.Lock()
			for key, value := range buckets {
				if now.Sub(value.start) > time.Minute {
					delete(buckets, key)
				}
			}
			key := ip + "|" + r.URL.Path
			value := buckets[key]
			if value.start.IsZero() {
				value.start = now
			}
			value.count++
			allowed := value.count <= 30 && len(buckets) < 10000
			buckets[key] = value
			mu.Unlock()
			if !allowed {
				writeError(w, 429, "操作过于频繁，请稍后重试")
				return
			}
		}
		if s.cfg.IdentitySecret != "" && !strings.Contains(r.URL.Path, "/client/") && r.Method != "GET" && r.Method != "HEAD" && r.Header.Get("Origin") != s.cfg.PublicURL {
			writeError(w, 403, "请从本站页面操作")
			return
		}
		next.ServeHTTP(w, r)
	})
}
