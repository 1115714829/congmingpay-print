package api

import (
	"net/http"
	"time"
)

// hDashboard 总览:四项统计 + 近 7 天(含今天)登录成功/失败。
func (s *Server) hDashboard(w http.ResponseWriter, r *http.Request) {
	m, inv, alloc, bound, err := s.Store.Counts()
	if err != nil {
		writeErr(w, http.StatusServiceUnavailable, 5000, "服务内部错误")
		return
	}
	stats, err := s.Store.LoginStats(7)
	if err != nil {
		writeErr(w, http.StatusServiceUnavailable, 5000, "服务内部错误")
		return
	}
	items := make([]map[string]interface{}, 0, 7)
	for i := 6; i >= 0; i-- {
		date := time.Now().UTC().AddDate(0, 0, -i).Format("2006-01-02")
		v := stats[date]
		items = append(items, map[string]interface{}{
			"date":    date,
			"success": v[0],
			"failed":  v[1],
		})
	}
	writeOK(w, map[string]interface{}{
		"merchantCount":  m,
		"inventoryCount": inv,
		"allocatedCount": alloc,
		"boundCount":     bound,
		"loginStats":     items,
	})
}
