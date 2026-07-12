package ginx

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// Access 路由访问级别；零值 AccessProtected 使「未声明即需登录」（secure-by-default）。
type Access uint8

const (
	AccessProtected Access = iota // 需登录（默认）
	AccessPublic                  // 放行
	AccessOptional                // 登录可选：验到写 claims，否则放行
)

// RouteRegistry 记录路由（method + 模板）的访问级别，供鉴权中间件按 ctx.FullPath() 查询。
type RouteRegistry struct {
	levels map[string]Access
}

func NewRouteRegistry() *RouteRegistry {
	return &RouteRegistry{levels: make(map[string]Access)}
}

// Lookup 返回访问级别；未登记返回 AccessProtected。
func (r *RouteRegistry) Lookup(method, path string) Access {
	return r.levels[method+" "+path]
}

func (r *RouteRegistry) set(method, path string, a Access) {
	r.levels[method+" "+path] = a
}

// Router 按访问级别注册路由：Router.GET/POST… 为 protected；
// Router.Public / Router.Optional 注册对应级别并登记进 RouteRegistry。
type Router struct {
	*scope
	Public   *scope
	Optional *scope
}

func NewRouter(engine *gin.Engine, reg *RouteRegistry) *Router {
	return &Router{
		scope:    &scope{engine, reg, AccessProtected},
		Public:   &scope{engine, reg, AccessPublic},
		Optional: &scope{engine, reg, AccessOptional},
	}
}

// scope 以固定访问级别注册路由。
type scope struct {
	engine *gin.Engine
	reg    *RouteRegistry
	level  Access
}

func (s *scope) handle(method, path string, h ...gin.HandlerFunc) {
	s.reg.set(method, path, s.level)
	s.engine.Handle(method, path, h...)
}

func (s *scope) GET(path string, h ...gin.HandlerFunc)    { s.handle(http.MethodGet, path, h...) }
func (s *scope) POST(path string, h ...gin.HandlerFunc)   { s.handle(http.MethodPost, path, h...) }
func (s *scope) PUT(path string, h ...gin.HandlerFunc)    { s.handle(http.MethodPut, path, h...) }
func (s *scope) DELETE(path string, h ...gin.HandlerFunc) { s.handle(http.MethodDelete, path, h...) }
func (s *scope) PATCH(path string, h ...gin.HandlerFunc)  { s.handle(http.MethodPatch, path, h...) }
