package sdk

import "strings"

// TrimRoute 规范化路由（去掉 /ws/、/api/ 前缀），与历史 WS router 字段兼容。
func TrimRoute(path string) string {
	p := strings.TrimSpace(path)
	p = strings.TrimPrefix(p, "/ws/")
	p = strings.TrimPrefix(p, "/api/")
	p = strings.TrimPrefix(p, "/")
	return p
}

func normalizeTarget(domain string) string {
	d := strings.TrimSpace(domain)
	for _, p := range []string{"grpc://", "http://", "https://", "ws://", "wss://"} {
		d = strings.TrimPrefix(d, p)
	}
	if i := strings.IndexByte(d, '/'); i >= 0 {
		d = d[:i]
	}
	return d
}
