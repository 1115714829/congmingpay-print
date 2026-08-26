// Package api 负责 HTTP 路由分发、鉴权中间件、响应信封与业务错误映射。
// 契约见 web/api/API.md(唯一标准)。
package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"congmingpay/web/internal/auth"
	"congmingpay/web/internal/store"
)

// 业务错误码(与 API.md 错误码总表一致)。
const (
	CodeOK           = 0
	CodeNotLogin     = 1001
	CodeNoPerm       = 1002
	CodeBadCred      = 1003
	CodeDisabled     = 1004
	CodeBadParam     = 2001
	CodeUsernameUsed = 2002
	CodeMerchantNone = 2003
	CodeMerchantDup  = 2004
	CodeNoAvail      = 3001
	CodeNotOwned     = 3002
	CodeBoundElse    = 3003
	CodeFpInvalid    = 3004
	CodeOccupied     = 3005
	CodeInsufficient = 3006
	CodeFileRead     = 4001
	CodeTmplBad      = 4002
	CodeDupInFile    = 4003
	CodeDupInStore   = 4004
)

// AppError 是业务错误(HTTP 200 + 业务码)。
type AppError struct {
	Code int
	Msg  string
}

func (e *AppError) Error() string { return e.Msg }

func appErr(code int, msg string) *AppError { return &AppError{Code: code, Msg: msg} }

// Server 持有依赖。
type Server struct {
	Store     *store.Store
	JWTSecret string
}

// Handler 构建 HTTP 路由(标准库 + 自写 {param} 分发)。
func (s *Server) Handler() http.Handler {
	routes := map[string]route{
		"POST /login":                                 {fn: s.hLogin},
		"GET /me":                                     {fn: s.hMe, auth: true},
		"PUT /me/password":                            {fn: s.hMePassword, auth: true},
		"GET /accounts":                               {fn: s.hListAccounts, auth: true, admin: true},
		"POST /accounts":                              {fn: s.hCreateAccount, auth: true, admin: true},
		"PUT /accounts/{id}":                          {fn: s.hUpdateAccount, auth: true, admin: true},
		"PUT /accounts/{id}/reset-password":           {fn: s.hResetPassword, auth: true, admin: true},
		"GET /merchants":                              {fn: s.hListMerchants, auth: true},
		"POST /merchants":                             {fn: s.hCreateMerchant, auth: true},
		"GET /merchants/{id}":                         {fn: s.hGetMerchant, auth: true},
		"DELETE /merchants/{id}":                      {fn: s.hDeleteMerchant, auth: true, admin: true},
		"POST /merchants/{id}/allocate":               {fn: s.hAllocate, auth: true},
		"GET /merchants/{id}/devices":                 {fn: s.hMerchantDevices, auth: true},
		"POST /merchants/{id}/devices/{name}/reclaim": {fn: s.hReclaim, auth: true},
		"GET /devices":                                {fn: s.hListDevices, auth: true},
		"GET /devices/{name}":                         {fn: s.hGetDevice, auth: true},
		"POST /devices/{name}/unbind":                 {fn: s.hUnbind, auth: true, admin: true},
		"POST /devices/batch-unbind":                  {fn: s.hBatchUnbind, auth: true, admin: true},
		"POST /devices/batch-reclaim":                 {fn: s.hBatchReclaim, auth: true, admin: true},
		"POST /inventory/import":                      {fn: s.hImport, auth: true, admin: true},
		"GET /inventory":                              {fn: s.hInventory, auth: true, admin: true},
		// 设备端接口:无 JWT,凭商户号+硬件指纹
		"POST /device/lookup": {fn: s.hLookup},
		"POST /device/bind":   {fn: s.hBind},
		"GET /dashboard":      {fn: s.hDashboard, auth: true},
	}
	mux := http.NewServeMux()
	apiHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/api/v1")
		if path == "" {
			path = "/"
		}
		rt, ok := matchRoute(routes, r.Method, path)
		if !ok {
			// 区分 404/405:同路径模式存在其他方法则 405
			var allow []string
			found := false
			for k := range routes {
				m, p, _ := strings.Cut(k, " ")
				if pathPattern(p, path) {
					found = true
					if m != r.Method {
						allow = append(allow, m)
					}
				}
			}
			if !found {
				writeJSON(w, http.StatusNotFound, envelope{Code: 4040, Message: "路径不存在", Data: nil})
				return
			}
			w.Header().Set("Allow", strings.Join(allow, ", "))
			writeJSON(w, http.StatusMethodNotAllowed, envelope{Code: 4050, Message: "方法不允许", Data: nil})
			return
		}
		s.dispatch(w, r, path, rt)
	})
	// 包装一层:每个请求落一行日志(方法 路径 http码 业务码 耗时)——排查"前端报了什么错"的依据
	mux.HandleFunc("/api/v1/", func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		lw := &logWriter{ResponseWriter: w, status: http.StatusOK}
		defer func() {
			code := -1
			if lw.buf.Len() > 0 {
				var env struct {
					Code int `json:"code"`
				}
				if json.Unmarshal(lw.buf.Bytes(), &env) == nil {
					code = env.Code
				}
			}
			log.Printf("API %s %s http=%d code=%d %s", r.Method, r.URL.Path, lw.status, code,
				time.Since(start).Round(time.Millisecond))
		}()
		apiHandler.ServeHTTP(lw, r)
	})
	return withCORS(mux)
}

// logWriter 捕获响应状态与正文,用于请求日志行。
type logWriter struct {
	http.ResponseWriter
	status int
	buf    bytes.Buffer
}

func (l *logWriter) WriteHeader(s int) {
	l.status = s
	l.ResponseWriter.WriteHeader(s)
}

func (l *logWriter) Write(b []byte) (int, error) {
	l.buf.Write(b)
	return l.ResponseWriter.Write(b)
}

type route struct {
	fn    func(w http.ResponseWriter, r *http.Request)
	auth  bool
	admin bool
}

// matchRoute 按「方法 + 路径模式」匹配路由;模式段 {param} 匹配任意单段。
func matchRoute(routes map[string]route, method, path string) (route, bool) {
	for k, rt := range routes {
		m, p, _ := strings.Cut(k, " ")
		if m == method && pathPattern(p, path) {
			return rt, true
		}
	}
	return route{}, false
}

// pathPattern 判断路径是否匹配模式(逐段比较,{param} 段通配)。
func pathPattern(pattern, path string) bool {
	pi := strings.Split(strings.Trim(pattern, "/"), "/")
	ci := strings.Split(strings.Trim(path, "/"), "/")
	if len(pi) != len(ci) {
		return false
	}
	for i, seg := range pi {
		if strings.HasPrefix(seg, "{") && strings.HasSuffix(seg, "}") {
			if ci[i] == "" {
				return false
			}
			continue
		}
		if seg != ci[i] {
			return false
		}
	}
	return true
}

// dispatch 鉴权 + 路径参数解析 + 执行 handler。
func (s *Server) dispatch(w http.ResponseWriter, r *http.Request, path string, rt route) {
	if rt.auth {
		tok := auth.Bearer(r.Header.Get("Authorization"))
		if tok == "" {
			w.Header().Set("WWW-Authenticate", "Bearer")
			writeErr(w, http.StatusUnauthorized, CodeNotLogin, "未登录或令牌过期")
			return
		}
		u, err := auth.Parse(s.JWTSecret, tok)
		if err != nil {
			w.Header().Set("WWW-Authenticate", "Bearer")
			code, msg := CodeNotLogin, "未登录或令牌过期"
			if errors.Is(err, auth.ErrInvalidToken) {
				msg = "无效令牌"
			}
			writeErr(w, http.StatusUnauthorized, code, msg)
			return
		}
		if rt.admin && u.Role != "admin" {
			writeErr(w, http.StatusOK, CodeNoPerm, "无权限(需admin)")
			return
		}
		ctx := r.Context()
		ctx = contextWithUser(ctx, u)
		r = r.WithContext(ctx)
	}
	rt.fn(w, r)
}

// relPath 剥掉 /api/v1 前缀与查询串,返回相对路径(如 /accounts/1)。
func relPath(r *http.Request) string {
	p := strings.TrimPrefix(r.URL.Path, "/api/v1")
	return p
}

// seg 返回相对路径第 i 段(0 基,跳过空前导段);越界返回空串。
// 例:rel=/accounts/1/reset-password → seg(0)=accounts seg(1)=1 seg(2)=reset-password
func seg(rel string, i int) string {
	parts := strings.Split(strings.Trim(rel, "/"), "/")
	if len(parts) == 1 && parts[0] == "" {
		return ""
	}
	if i < 0 || i >= len(parts) {
		return ""
	}
	return parts[i]
}

// segID 返回路径段中的正整数;非法返回 0。
func segID(rel string, i int) int64 {
	v := seg(rel, i)
	if v == "" {
		return 0
	}
	var n int64
	for _, c := range v {
		if c < '0' || c > '9' {
			return 0
		}
		n = n*10 + int64(c-'0')
	}
	if n <= 0 {
		return 0
	}
	return n
}

// ---- 响应信封 ----

type envelope struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data"`
}

func writeOK(w http.ResponseWriter, data interface{}) {
	writeJSON(w, http.StatusOK, envelope{Code: CodeOK, Message: "ok", Data: data})
}

func writeErr(w http.ResponseWriter, httpStatus, code int, msg string) {
	writeJSON(w, httpStatus, envelope{Code: code, Message: msg, Data: nil})
}

// writeResult 是 handler 的统一出口:AppError 按业务码,其余按 500。
func writeResult(w http.ResponseWriter, err error, data interface{}) {
	if err == nil {
		writeOK(w, data)
		return
	}
	var ae *AppError
	if errors.As(err, &ae) {
		writeErr(w, http.StatusOK, ae.Code, ae.Msg)
		return
	}
	writeErr(w, http.StatusServiceUnavailable, 5000, "服务内部错误")
}

func writeJSON(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func readJSON(r *http.Request, v interface{}) error {
	defer r.Body.Close()
	dec := json.NewDecoder(r.Body)
	return dec.Decode(v)
}

// clientIP 取对端 IP(简单处理 X-Forwarded-For 首段)。
func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		return strings.TrimSpace(strings.Split(xff, ",")[0])
	}
	host := r.RemoteAddr
	if i := strings.LastIndex(host, ":"); i > 0 {
		return host[:i]
	}
	return host
}

// ---- CORS(开发期放行 Vite) ----

func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		origin := r.Header.Get("Origin")
		if origin == "http://localhost:5173" || origin == "http://127.0.0.1:5173" ||
			origin == "http://localhost:5174" || origin == "http://127.0.0.1:5174" {
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Methods", "GET,POST,PUT,DELETE,OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Authorization,Content-Type,X-Device-Name")
			w.Header().Set("Access-Control-Max-Age", "86400")
		}
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// ---- 上下文 ----

type contextKey int

const userKey contextKey = 0

func contextWithUser(ctx context.Context, u auth.User) context.Context {
	return context.WithValue(ctx, userKey, u)
}

// userFrom 取请求上下文中的登录用户(仅管理端接口已注入)。
func userFrom(ctx context.Context) (auth.User, bool) {
	u, ok := ctx.Value(userKey).(auth.User)
	return u, ok
}
