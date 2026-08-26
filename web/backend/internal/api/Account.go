package api

import (
	"net/http"

	"congmingpay/web/internal/auth"
	"congmingpay/web/internal/store"
)

// UserVO 账号视图(不下发密码)。
type UserVO struct {
	ID          int64  `json:"id"`
	Username    string `json:"username"`
	DisplayName string `json:"displayName"`
	Role        string `json:"role"`
	Enabled     bool   `json:"enabled"`
	CreatedAt   string `json:"createdAt"`
}

func userVO(a *store.Account) UserVO {
	return UserVO{a.ID, a.Username, a.DisplayName, a.Role, a.Enabled, a.CreatedAt}
}

func mustHash(p string) string {
	h, _ := auth.HashPassword(p)
	return h
}

// validPassword 6-64 位且含字母数字。
func validPassword(p string) bool {
	if len(p) < 6 || len(p) > 64 {
		return false
	}
	var hasAlpha, hasNum bool
	for _, c := range p {
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z':
			hasAlpha = true
		case c >= '0' && c <= '9':
			hasNum = true
		}
	}
	return hasAlpha && hasNum
}

func validUsername(s string) bool {
	if len(s) < 2 || len(s) > 32 {
		return false
	}
	for _, c := range s {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_') {
			return false
		}
	}
	return true
}

func atoiParam(s string) int {
	if s == "" {
		return 0
	}
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0
		}
		n = n*10 + int(c-'0')
	}
	return n
}

// ---- 登录/自身 ----

func (s *Server) hLogin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := readJSON(r, &req); err != nil || req.Username == "" || req.Password == "" {
		writeErr(w, http.StatusOK, CodeBadParam, "参数错误")
		return
	}
	a, err := s.Store.GetAccountByUsername(req.Username)
	if err != nil || !auth.VerifyPassword(a.PasswordHash, req.Password) {
		s.Store.WriteLoginLog(req.Username, clientIP(r), r.UserAgent(), false, "用户名或密码错误")
		writeErr(w, http.StatusOK, CodeBadCred, "用户名或密码错误")
		return
	}
	if !a.Enabled {
		s.Store.WriteLoginLog(req.Username, clientIP(r), r.UserAgent(), false, "账号已停用")
		writeErr(w, http.StatusOK, CodeDisabled, "账号已停用")
		return
	}
	s.Store.WriteLoginLog(req.Username, clientIP(r), r.UserAgent(), true, "")
	tok, err := auth.Issue(s.JWTSecret, auth.User{ID: a.ID, Name: a.Username, Role: a.Role})
	if err != nil {
		writeErr(w, http.StatusServiceUnavailable, 5000, "令牌签发失败")
		return
	}
	writeOK(w, map[string]interface{}{"token": tok, "user": userVO(a)})
}

func (s *Server) hMe(w http.ResponseWriter, r *http.Request) {
	u, _ := userFrom(r.Context())
	a, err := s.Store.GetAccountByID(u.ID)
	if err != nil {
		writeErr(w, http.StatusUnauthorized, CodeNotLogin, "未登录或令牌过期")
		return
	}
	writeOK(w, userVO(a))
}

func (s *Server) hMePassword(w http.ResponseWriter, r *http.Request) {
	var req struct {
		OldPassword string `json:"oldPassword"`
		NewPassword string `json:"newPassword"`
	}
	if err := readJSON(r, &req); err != nil || req.OldPassword == "" || !validPassword(req.NewPassword) {
		writeErr(w, http.StatusOK, CodeBadParam, "参数错误")
		return
	}
	u, _ := userFrom(r.Context())
	a, err := s.Store.GetAccountByID(u.ID)
	if err != nil || !auth.VerifyPassword(a.PasswordHash, req.OldPassword) {
		writeErr(w, http.StatusOK, CodeBadCred, "原密码错误")
		return
	}
	if err := s.Store.SetPassword(a.ID, mustHash(req.NewPassword)); err != nil {
		writeErr(w, http.StatusServiceUnavailable, 5000, "服务内部错误")
		return
	}
	writeOK(w, nil)
}

// ---- 账号管理(admin) ----

func (s *Server) hListAccounts(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	total, list, err := s.Store.ListAccounts(atoiParam(q.Get("page")), atoiParam(q.Get("pageSize")), q.Get("keyword"))
	if err != nil {
		writeErr(w, http.StatusServiceUnavailable, 5000, "服务内部错误")
		return
	}
	items := make([]UserVO, 0, len(list))
	for _, a := range list {
		items = append(items, userVO(a))
	}
	writeOK(w, map[string]interface{}{"total": total, "items": items})
}

func (s *Server) hCreateAccount(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Username    string `json:"username"`
		Password    string `json:"password"`
		DisplayName string `json:"displayName"`
		Role        string `json:"role"`
	}
	if err := readJSON(r, &req); err != nil {
		writeErr(w, http.StatusOK, CodeBadParam, "参数错误")
		return
	}
	if !validUsername(req.Username) || !validPassword(req.Password) || (req.Role != "admin" && req.Role != "operator") {
		writeErr(w, http.StatusOK, CodeBadParam, "参数错误")
		return
	}
	a, err := s.Store.CreateAccount(req.Username, mustHash(req.Password), req.DisplayName, req.Role)
	switch {
	case err == nil:
		writeOK(w, userVO(a))
	case err == store.ErrUsernameUsed:
		writeErr(w, http.StatusOK, CodeUsernameUsed, "用户名已存在")
	default:
		writeErr(w, http.StatusServiceUnavailable, 5000, "服务内部错误")
	}
}

func (s *Server) hUpdateAccount(w http.ResponseWriter, r *http.Request) {
	id := segID(relPath(r), 1)
	if id <= 0 {
		writeErr(w, http.StatusOK, CodeBadParam, "参数错误")
		return
	}
	var req struct {
		DisplayName *string `json:"displayName"`
		Role        *string `json:"role"`
		Enabled     *bool   `json:"enabled"`
	}
	if err := readJSON(r, &req); err != nil {
		writeErr(w, http.StatusOK, CodeBadParam, "参数错误")
		return
	}
	a, err := s.Store.UpdateAccount(id, req.DisplayName, req.Role, req.Enabled)
	switch {
	case err == nil:
		writeOK(w, userVO(a))
	case err == store.ErrNotFound:
		writeErr(w, http.StatusOK, CodeBadParam, "参数错误")
	default:
		writeErr(w, http.StatusServiceUnavailable, 5000, "服务内部错误")
	}
}

func (s *Server) hResetPassword(w http.ResponseWriter, r *http.Request) {
	id := segID(relPath(r), 1)
	if id <= 0 {
		writeErr(w, http.StatusOK, CodeBadParam, "参数错误")
		return
	}
	var req struct {
		NewPassword string `json:"newPassword"`
	}
	if err := readJSON(r, &req); err != nil || !validPassword(req.NewPassword) {
		writeErr(w, http.StatusOK, CodeBadParam, "参数错误")
		return
	}
	if err := s.Store.SetPassword(id, mustHash(req.NewPassword)); err != nil {
		writeErr(w, http.StatusServiceUnavailable, 5000, "服务内部错误")
		return
	}
	writeOK(w, nil)
}
