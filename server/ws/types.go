package wssvr

import (
	"net"
	"net/http"
	"strings"
	"sync"

	"github.com/godaddy-x/freego/core/crypto"
	cache "github.com/godaddy-x/freego/infra/cache/contract"
	"github.com/godaddy-x/freego/infra/cachelocal"
	fwsign "github.com/godaddy-x/freego/protocol/sign"
)

var defaultCacheObject = cachelocal.NewDefaultLocalCache()

var messageHandlerPool = sync.Pool{
	New: func() interface{} {
		return &MessageHandler{}
	},
}

func GetMessageHandler(hook fwsign.CipherHook, handle Handle) *MessageHandler {
	mh := messageHandlerPool.Get().(*MessageHandler)
	mh.cipherHook = hook
	mh.handle = handle
	return mh
}

func PutMessageHandler(mh *MessageHandler) {
	if mh == nil {
		return
	}
	mh.handle = nil
	mh.cipherHook = nil
	messageHandlerPool.Put(mh)
}

type CacheAware func(ds ...string) (cache.Cache, error)

type RouterConfig struct {
	Guest       bool
	UsePlan2    bool
	KeyRoute    bool
	LoginRoute  bool
	AesRequest  bool
	AesResponse bool
}

func (self *WsServer) getPQCipher(usr int64) (crypto.Cipher, error) {
	return fwsign.ResolvePQCipher(self.cipherHook, usr)
}

func (mh *MessageHandler) getPQCipher(usr int64) (crypto.Cipher, error) {
	return fwsign.ResolvePQCipher(mh.cipherHook, usr)
}

func RemoteIPFromRequest(r *http.Request) string {
	if r == nil {
		return ""
	}
	fallback := r.RemoteAddr
	if host, _, err := net.SplitHostPort(fallback); err == nil {
		fallback = host
	}
	return resolveRemoteIP(
		r.Header.Get("CF-Connecting-IP"),
		r.Header.Get("X-Forwarded-For"),
		r.Header.Get("X-Real-Ip"),
		fallback,
	)
}

func resolveRemoteIP(cfConnectingIP, xffHeader, realIPHeader, fallback string) string {
	cfConnectingIP = strings.TrimSpace(cfConnectingIP)
	if cfConnectingIP != "" {
		ip := net.ParseIP(cfConnectingIP)
		if ip != nil && !isPrivateIP(ip) {
			return ip.String()
		}
	}
	xffHeader = strings.TrimSpace(xffHeader)
	if xffHeader != "" {
		ips := strings.Split(xffHeader, ",")
		for i := len(ips) - 1; i >= 0; i-- {
			ipStr := strings.TrimSpace(ips[i])
			if ipStr == "" {
				continue
			}
			ip := net.ParseIP(ipStr)
			if ip != nil && !isPrivateIP(ip) {
				return ip.String()
			}
		}
	}
	realIPHeader = strings.TrimSpace(realIPHeader)
	if realIPHeader != "" {
		ip := net.ParseIP(realIPHeader)
		if ip != nil && !isPrivateIP(ip) {
			return ip.String()
		}
	}
	fallback = strings.TrimSpace(fallback)
	if fallback != "" {
		ip := net.ParseIP(fallback)
		if ip != nil && !isPrivateIP(ip) {
			return fallback
		}
	}
	return ""
}

var privateIPBlocks = []*net.IPNet{
	{IP: net.ParseIP("10.0.0.0"), Mask: net.CIDRMask(8, 32)},
	{IP: net.ParseIP("172.16.0.0"), Mask: net.CIDRMask(12, 32)},
	{IP: net.ParseIP("192.168.0.0"), Mask: net.CIDRMask(16, 32)},
	{IP: net.ParseIP("127.0.0.0"), Mask: net.CIDRMask(8, 32)},
	{IP: net.ParseIP("0.0.0.0"), Mask: net.CIDRMask(8, 32)},
}

func isPrivateIP(ip net.IP) bool {
	if ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return true
	}
	for _, block := range privateIPBlocks {
		if block.Contains(ip) {
			return true
		}
	}
	return false
}
