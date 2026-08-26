package api

import (
	"net/http"

	"congmingpay/web/internal/store"
)

// DeviceVO 设备视图(设备页表格列来源)。
type DeviceVO struct {
	Name              string  `json:"name"`
	DeviceSecret      string  `json:"deviceSecret"`
	ProductKey        string  `json:"productKey"`
	MerchantID        *int64  `json:"merchantId"`
	MerchantNoLong    *string `json:"merchantNoLong"`
	MerchantNoShort   *string `json:"merchantNoShort"`
	MerchantName      *string `json:"merchantName"`
	State             string  `json:"state"`
	AllocatedAt       *string `json:"allocatedAt"`
	AllocatedLeftDays int     `json:"allocatedLeftDays"`
	BoundAt           *string `json:"boundAt"`
	LastSeenAt        *string `json:"lastSeenAt"`
	Online            bool    `json:"online"`
	OsType            *string `json:"osType"`
	AppVersion        *string `json:"appVersion"`
	CreatedAt         string  `json:"createdAt"`
}

func deviceVO(d *store.Device) DeviceVO {
	v := DeviceVO{
		Name:              d.Name,
		DeviceSecret:      d.DeviceSecret,
		ProductKey:        d.ProductKey,
		State:             d.State(),
		AllocatedLeftDays: d.AllocatedLeftDays,
		Online:            d.Online,
		CreatedAt:         d.CreatedAt,
	}
	if d.MerchantID.Valid {
		v.MerchantID = &d.MerchantID.Int64
	}
	if d.MerchantNoLong.Valid {
		v.MerchantNoLong = &d.MerchantNoLong.String
	}
	if d.MerchantNoShort.Valid {
		v.MerchantNoShort = &d.MerchantNoShort.String
	}
	if d.MerchantName.Valid {
		v.MerchantName = &d.MerchantName.String
	}
	if d.AllocatedAt.Valid {
		v.AllocatedAt = &d.AllocatedAt.String
	}
	if d.BoundAt.Valid {
		v.BoundAt = &d.BoundAt.String
	}
	if d.LastSeenAt.Valid {
		v.LastSeenAt = &d.LastSeenAt.String
	}
	if d.OsType.Valid {
		v.OsType = &d.OsType.String
	}
	if d.AppVersion.Valid {
		v.AppVersion = &d.AppVersion.String
	}
	return v
}

func deviceVOs(list []*store.Device) []DeviceVO {
	out := make([]DeviceVO, 0, len(list))
	for _, d := range list {
		out = append(out, deviceVO(d))
	}
	return out
}

func (s *Server) hListDevices(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	total, list, err := s.Store.ListDevices(
		atoiParam(q.Get("page")), atoiParam(q.Get("pageSize")),
		q.Get("keyword"), q.Get("state"),
		int64(atoiParam(q.Get("merchantId"))))
	if err != nil {
		writeErr(w, http.StatusServiceUnavailable, 5000, "服务内部错误")
		return
	}
	writeOK(w, map[string]interface{}{"total": total, "items": deviceVOs(list)})
}

// DeviceDetailVO 设备详情(含绑定指纹 raw)。
type DeviceDetailVO struct {
	DeviceVO
	Merchant    *MerchantVO `json:"merchant"`
	Fingerprint string      `json:"fingerprint"`
}

func (s *Server) hGetDevice(w http.ResponseWriter, r *http.Request) {
	name := seg(relPath(r), 1) // /devices/{name}
	if name == "" {
		writeErr(w, http.StatusOK, CodeBadParam, "参数错误")
		return
	}
	d, raw, err := s.Store.GetDevice(name)
	if err == store.ErrNotFound {
		writeErr(w, http.StatusOK, CodeNotOwned, "设备不存在")
		return
	}
	if err != nil {
		writeErr(w, http.StatusServiceUnavailable, 5000, "服务内部错误")
		return
	}
	v := DeviceDetailVO{DeviceVO: deviceVO(d), Fingerprint: raw}
	if d.MerchantID.Valid {
		if m, err := s.Store.GetMerchantByID(d.MerchantID.Int64); err == nil {
			v.Merchant = &MerchantVO{m.ID, m.MerchantNoLong, m.MerchantNoShort, m.Name,
				m.ContactPhone, m.Address, m.Remark, m.AllocatedCount, m.BoundCount, m.CreatedAt}
		}
	}
	writeOK(w, v)
}

func (s *Server) hUnbind(w http.ResponseWriter, r *http.Request) {
	name := seg(relPath(r), 1) // /devices/{name}/unbind
	if name == "" {
		writeErr(w, http.StatusOK, CodeBadParam, "参数错误")
		return
	}
	err := s.Store.Unbind(name)
	switch {
	case err == nil:
		writeOK(w, nil)
	case err == store.ErrNotBound:
		writeErr(w, http.StatusOK, CodeBadParam, "设备未绑定,不可解绑")
	default:
		writeErr(w, http.StatusServiceUnavailable, 5000, "服务内部错误")
	}
}

func (s *Server) hBatchUnbind(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Names []string `json:"names"`
	}
	if err := readJSON(r, &req); err != nil || len(req.Names) == 0 || len(req.Names) > 200 {
		writeErr(w, http.StatusOK, CodeBadParam, "参数错误")
		return
	}
	unbound, skipped := s.Store.BatchUnbind(req.Names)
	writeOK(w, map[string]interface{}{"unbound": unbound, "skipped": skipped})
}

func (s *Server) hBatchReclaim(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Names []string `json:"names"`
	}
	if err := readJSON(r, &req); err != nil || len(req.Names) == 0 || len(req.Names) > 200 {
		writeErr(w, http.StatusOK, CodeBadParam, "参数错误")
		return
	}
	reclaimed, skipped := s.Store.BatchReclaim(req.Names)
	writeOK(w, map[string]interface{}{"reclaimed": reclaimed, "skipped": skipped})
}
