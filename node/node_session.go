package node

import (
	"github.com/godaddy-x/freego/cache"
	"github.com/godaddy-x/freego/component/jwt"
	"github.com/godaddy-x/freego/util"
)

type Session interface {
	GetId() string

	GetStartTimestamp() int64

	GetLastAccessTime() int64

	GetTimeout() (int64, error) // 抛出校验会话异常

	SetTimeout(t int64) error // 抛出校验会话异常

	SetAccessToken(token string) error // 抛出校验会话异常

	SetHost(host string)

	GetHost() string

	Touch() error // 刷新最后授权时间,抛出校验会话异常

	Stop() error // 抛出校验会话异常

	GetAttributeKeys() ([]string, error) // 抛出校验会话异常

	GetAttribute(k string) (interface{}, error) // 抛出校验会话异常

	SetAttribute(k string, v interface{}) error // 抛出校验会话异常

	RemoveAttribute(k string) error // 抛出校验会话异常

	Validate() error // 校验会话

	IsValid() bool // 判断会话是否有效

	IsTimeout() bool // 判断会话是否已超时
}

type SessionAware interface {
	CreateSession(s Session) error // 保存session

	ReadSession(s string) (Session, error) // 通过id读取session,抛出未知session异常

	UpdateSession(s Session) error // 更新session,抛出未知session异常

	DeleteSession(s Session) error // 删除session,抛出未知session异常

	GetActiveSessions() []Session // 获取活动的session集合
}

type JWTSession struct {
	Id             string
	StartTimestamp int64
	LastAccessTime int64
	Timeout        int64
	StopTime       int64
	Host           string
	Expire         bool
	Attributes     map[string]interface{}
	AccessToken    string
}

func (self *JWTSession) Build(sub *jwt.Subject, srt string) (*jwt.Authorization, error) {
	author, err := sub.Generate(srt);
	if err != nil {
		return nil, err
	}
	self.Id = author.Signature
	self.Host = sub.Payload.Aud
	self.Timeout = sub.Payload.Exp
	self.StartTimestamp = author.AccessTime
	self.LastAccessTime = author.AccessTime
	self.AccessToken = author.AccessToken
	self.Attributes = map[string]interface{}{}
	return author, nil
}

func (self *JWTSession) GetId() string {
	return self.Id
}

func (self *JWTSession) GetStartTimestamp() int64 {
	return self.StartTimestamp
}

func (self *JWTSession) GetLastAccessTime() int64 {
	return self.LastAccessTime
}

func (self *JWTSession) GetTimeout() (int64, error) {
	if err := self.Validate(); err != nil {
		return 0, err
	}
	return self.Timeout, nil
}

func (self *JWTSession) SetTimeout(t int64) error {
	self.Timeout = t
	return nil
}

func (self *JWTSession) SetAccessToken(token string) error {
	self.AccessToken = token
	return nil
}

func (self *JWTSession) SetHost(host string) {
	self.Host = host
}

func (self *JWTSession) GetHost() string {
	return self.Host
}

func (self *JWTSession) Touch() error {
	self.LastAccessTime = util.Time()
	return nil
}

func (self *JWTSession) Stop() error {
	self.StopTime = util.Time()
	self.Expire = true
	return nil
}

func (self *JWTSession) Validate() error {
	if self.Expire {
		return util.Error("session[", self.Id, "] expired")
	} else if self.IsTimeout() {
		return util.Error("session[", self.Id, "] timeout invalid")
	}
	// JWT二次校验
	subject := &jwt.Subject{}
	if len(self.GetHost()) > 0 {
		subject.Payload = &jwt.Payload{Aud: self.GetHost()}
	}
	if err := subject.Valid(self.AccessToken, Global.SessionSecret); err != nil {
		return err
	}
	return nil
}

func (self *JWTSession) IsValid() bool {
	if len(self.Id) > 0 && !self.Expire && self.StopTime == 0 {
		return true
	}
	return false
}

func (self *JWTSession) IsTimeout() bool {
	if util.Time() > (self.LastAccessTime + self.Timeout) {
		self.Stop()
		return true
	}
	return false
}

func (self *JWTSession) GetAttributeKeys() ([]string, error) {
	keys := []string{}
	for k, _ := range self.Attributes {
		keys = append(keys, k)
	}
	return keys, nil
}

func (self *JWTSession) GetAttribute(k string) (interface{}, error) {
	if len(k) == 0 {
		return nil, nil
	}
	if v, b := self.Attributes[k]; b {
		return v, nil
	}
	return nil, nil
}

func (self *JWTSession) SetAttribute(k string, v interface{}) error {
	self.Attributes[k] = v
	return nil
}

func (self *JWTSession) RemoveAttribute(k string) error {
	if len(k) == 0 {
		return nil
	}
	if _, b := self.Attributes[k]; b {
		delete(self.Attributes, k)
	}
	return nil
}

/********************************* Session Cache Impl *********************************/

func NewLocalCacheSessionAware() *DefaultCacheSessionAware {
	return &DefaultCacheSessionAware{
		c: new(cache.LocalMapManager).NewCache(30, 10),
	}
}

type DefaultCacheSessionAware struct {
	c cache.ICache
}

func (self *DefaultCacheSessionAware) CreateSession(s Session) error {
	if !s.IsValid() {
		return util.Error("session[", s.GetId(), "] create invalid")
	}
	if err := s.Touch(); err != nil {
		return err
	}
	if err := s.Validate(); err != nil {
		return err
	}
	s.SetAccessToken("")
	self.c.Put(s.GetId(), s, Global.SessionTimeout)
	return nil
}

func (self *DefaultCacheSessionAware) ReadSession(s string) (Session, error) {
	var session Session
	if v, b, err := self.c.Get(s, session); err != nil {
		return nil, util.Error("session[", s, "] read err: ", err.Error())
	} else if b && v != nil {
		if r, ok := v.(Session); ok {
			if !r.IsValid() {
				return nil, util.Error("session[", s, "] read invalid")
			}
			return r, nil
		}
	}
	return nil, nil
}

func (self *DefaultCacheSessionAware) UpdateSession(s Session) error {
	if err := s.Touch(); err != nil {
		return err
	}
	self.c.Put(s.GetId(), s, Global.SessionTimeout)
	return nil
}

func (self *DefaultCacheSessionAware) DeleteSession(s Session) error {
	if s == nil {
		return nil
	}
	s.Stop()
	self.c.Del(s.GetId())
	return nil
}

func (self *DefaultCacheSessionAware) GetActiveSessions() []Session {
	return nil
}
