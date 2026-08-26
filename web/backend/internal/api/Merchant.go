package api

import (
	"net/http"

	"congmingpay/web/internal/store"
)

// MerchantVO 商户视图。
type MerchantVO struct {
	ID              int64  `json:"id"`
	MerchantNoLong  string `json:"merchantNoLong"`
	MerchantNoShort string `json:"merchantNoShort"`
	Name            string `json:"name"`
	ContactPhone    string `json:"contactPhone"`
	Address         string `json:"address"`
	Remark          string `json:"remark"`
	AllocatedCount  int    `json:"allocatedCount"`
	BoundCount      int    `json:"boundCount"`
	CreatedAt       string `json:"createdAt"`
}

func merchantVO(m *store.Merchant) MerchantVO {
	return MerchantVO{m.ID, m.MerchantNoLong, m.MerchantNoShort, m.Name, m.ContactPhone,
		m.Address, m.Remark, m.AllocatedCount, m.BoundCount, m.CreatedAt}
}

func (s *Server) hListMerchants(w http.ResponseWriter, r *http.Request) {
	list, err := s.Store.ListMerchants()
	if err != nil {
		writeErr(w, http.StatusServiceUnavailable, 5000, "服务内部错误")
		return
	}
	total := len(list)
	page, pageSize := atoiParam(r.URL.Query().Get("page")), atoiParam(r.URL.Query().Get("pageSize"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 50
	}
	start := (page - 1) * pageSize
	if start > total {
		start = total
	}
	end := start + pageSize
	if end > total {
		end = total
	}
	items := make([]MerchantVO, 0, end-start)
	for _, m := range list[start:end] {
		items = append(items, merchantVO(m))
	}
	writeOK(w, map[string]interface{}{"total": total, "items": items})
}

func (s *Server) hCreateMerchant(w http.ResponseWriter, r *http.Request) {
	var req struct {
		MerchantNoLong  string `json:"merchantNoLong"`
		MerchantNoShort string `json:"merchantNoShort"`
		Name            string `json:"name"`
		ContactPhone    string `json:"contactPhone"`
		Address         string `json:"address"`
		Remark          string `json:"remark"`
	}
	if err := readJSON(r, &req); err != nil {
		writeErr(w, http.StatusOK, CodeBadParam, "参数错误")
		return
	}
	if !validNo(req.MerchantNoLong) || !validNo(req.MerchantNoShort) ||
		req.Name == "" || len(req.Name) > 64 {
		writeErr(w, http.StatusOK, CodeBadParam, "参数错误")
		return
	}
	u, _ := userFrom(r.Context())
	m, err := s.Store.CreateMerchant(req.MerchantNoLong, req.MerchantNoShort, req.Name,
		req.ContactPhone, req.Address, req.Remark, u.Name)
	switch {
	case err == nil:
		writeOK(w, merchantVO(m))
	case err == store.ErrNoLongUsed:
		writeErr(w, http.StatusOK, CodeMerchantDup, "商户号(长)已存在")
	case err == store.ErrNoShortUsed:
		writeErr(w, http.StatusOK, CodeMerchantDup, "商户号(短)已存在")
	default:
		writeErr(w, http.StatusServiceUnavailable, 5000, "服务内部错误")
	}
}

func (s *Server) hGetMerchant(w http.ResponseWriter, r *http.Request) {
	id := segID(relPath(r), 1)
	m, err := s.Store.GetMerchantByID(id)
	if err != nil {
		writeErr(w, http.StatusOK, CodeBadParam, "参数错误")
		return
	}
	writeOK(w, merchantVO(m))
}

func (s *Server) hDeleteMerchant(w http.ResponseWriter, r *http.Request) {
	id := segID(relPath(r), 1)
	if id <= 0 {
		writeErr(w, http.StatusOK, CodeBadParam, "参数错误")
		return
	}
	released, err := s.Store.DeleteMerchant(id)
	switch {
	case err == nil:
		writeOK(w, map[string]interface{}{"released": released})
	case err == store.ErrNotFound:
		writeErr(w, http.StatusOK, CodeBadParam, "商户不存在")
	case err == store.ErrStillBound:
		writeErr(w, http.StatusOK, CodeBadParam, "该商户名下有已绑定设备,请先解绑再删除")
	default:
		writeErr(w, http.StatusServiceUnavailable, 5000, "服务内部错误")
	}
}

// validNo 商户号:1-32 位字母数字下划线。
func validNo(s string) bool {
	if len(s) < 1 || len(s) > 32 {
		return false
	}
	for _, c := range s {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '_') {
			return false
		}
	}
	return true
}

// ---- 商户设备:分配/收回 ----

func (s *Server) hAllocate(w http.ResponseWriter, r *http.Request) {
	id := segID(relPath(r), 1)
	if id <= 0 {
		writeErr(w, http.StatusOK, CodeBadParam, "参数错误")
		return
	}
	var req struct {
		Count int `json:"count"`
	}
	if err := readJSON(r, &req); err != nil || req.Count <= 0 {
		writeErr(w, http.StatusOK, CodeBadParam, "参数错误")
		return
	}
	names, err := s.Store.Allocate(id, req.Count)
	switch {
	case err == nil:
		writeOK(w, map[string]interface{}{"allocated": names})
	case err == store.ErrInsufficient:
		writeErr(w, http.StatusOK, CodeInsufficient, "库存不足")
	default:
		writeErr(w, http.StatusServiceUnavailable, 5000, "服务内部错误")
	}
}

func (s *Server) hMerchantDevices(w http.ResponseWriter, r *http.Request) {
	id := segID(relPath(r), 1)
	list, err := s.Store.ListMerchantDevices(id)
	if err != nil {
		writeErr(w, http.StatusServiceUnavailable, 5000, "服务内部错误")
		return
	}
	writeOK(w, map[string]interface{}{"items": deviceVOs(list)})
}

func (s *Server) hReclaim(w http.ResponseWriter, r *http.Request) {
	id := segID(relPath(r), 1)
	name := seg(relPath(r), 3) // /merchants/{id}/devices/{name}/reclaim → 段 3 为 name
	if id <= 0 || name == "" {
		writeErr(w, http.StatusOK, CodeBadParam, "参数错误")
		return
	}
	err := s.Store.Reclaim(id, name)
	switch {
	case err == nil:
		writeOK(w, nil)
	case err == store.ErrStillBound:
		writeErr(w, http.StatusOK, CodeBadParam, err.Error())
	case err == store.ErrNotOwned:
		writeErr(w, http.StatusOK, CodeNotOwned, err.Error())
	default:
		writeErr(w, http.StatusServiceUnavailable, 5000, "服务内部错误")
	}
}
