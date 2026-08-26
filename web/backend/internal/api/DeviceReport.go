package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"congmingpay/web/internal/fingerprint"
	"congmingpay/web/internal/store"
)

// DeviceInfo 设备端自报的展示信息(osType:win/android)。
// osType/appVersion 写入 devices 表,设备页「平台/版本」列来源。
type DeviceInfo struct {
	OsType      string `json:"osType"`
	AppVersion  string `json:"appVersion"`
	OsBuild     string `json:"osBuild"`
	DeviceModel string `json:"deviceModel"`
}

// deviceOpt 是 lookup 返回的候选设备(名称+密钥,客户端据此回填本地 MQTT 配置;
// 绑定确认推迟到客户端「保存设置」时的 /device/bind 调用)。
type deviceOpt struct {
	Name         string `json:"name"`
	ProductKey   string `json:"productKey"`
	DeviceSecret string `json:"deviceSecret"`
}

// deviceInfoOf 合并 deviceInfo 与指纹内同名字段(deviceInfo 缺省时回退指纹)。
func deviceInfoOf(fp *fingerprint.Fingerprint, di *DeviceInfo) DeviceInfo {
	out := DeviceInfo{}
	if di != nil {
		out = *di
	}
	if fp == nil {
		return out
	}
	if out.OsType == "" {
		out.OsType = fp.OsType
	}
	if out.AppVersion == "" {
		out.AppVersion = fp.AppVersion
	}
	return out
}

// hLookup 设备端查询可认领设备 / 重装恢复探测(无 JWT,凭商户号+硬件指纹)。
// 只做查询与心跳,不产生绑定;绑定由客户端在「保存设置」后调 /device/bind 上报确认。
func (s *Server) hLookup(w http.ResponseWriter, r *http.Request) {
	var req struct {
		MerchantNo  string                  `json:"merchantNo"`
		Fingerprint fingerprint.Fingerprint `json:"fingerprint"`
		DeviceInfo  *DeviceInfo             `json:"deviceInfo"`
	}
	if err := readJSON(r, &req); err != nil || req.MerchantNo == "" {
		writeErr(w, http.StatusOK, CodeBadParam, "参数错误")
		return
	}
	m, err := s.Store.FindMerchantByNo(req.MerchantNo)
	if err != nil {
		writeErr(w, http.StatusOK, CodeMerchantNone, "商户号不存在")
		return
	}
	if err := req.Fingerprint.Validate(); err != nil {
		writeErr(w, http.StatusOK, CodeFpInvalid, "设备指纹无效")
		return
	}
	fpHash := req.Fingerprint.Hash()
	boundName, err := s.Store.FindBoundDeviceByFP(fpHash)
	if err != nil {
		writeErr(w, http.StatusServiceUnavailable, 5000, "服务内部错误")
		return
	}
	// 本机指纹已绑定设备(重装恢复):返回名称+密钥供客户端回填配置
	var boundDev *deviceOpt
	if boundName != "" {
		if d, _, err := s.Store.GetDevice(boundName); err == nil {
			boundDev = &deviceOpt{Name: d.Name, ProductKey: d.ProductKey, DeviceSecret: d.DeviceSecret}
		}
	}
	devs, err := s.Store.ListMerchantDevices(m.ID)
	if err != nil {
		writeErr(w, http.StatusServiceUnavailable, 5000, "服务内部错误")
		return
	}
	avail := make([]deviceOpt, 0, len(devs))
	boundCount := 0
	for _, d := range devs {
		if d.BoundFPHash.Valid {
			boundCount++
			continue
		}
		avail = append(avail, deviceOpt{Name: d.Name, ProductKey: d.ProductKey, DeviceSecret: d.DeviceSecret})
	}
	// 心跳:刷新指纹/设备 last_seen 与 os_type/app_version(失败不阻塞查询)
	di := deviceInfoOf(&req.Fingerprint, req.DeviceInfo)
	_ = s.Store.TouchSeen(fpHash, di.OsType, di.AppVersion)

	data := map[string]interface{}{
		"merchant": map[string]string{
			"merchantNoLong":  m.MerchantNoLong,
			"merchantNoShort": m.MerchantNoShort,
			"name":            m.Name,
		},
		"boundDevice":      boundDev,
		"availableDevices": avail,
		"total":            len(avail),
		"boundCount":       boundCount,
	}
	if boundDev == nil && len(avail) == 0 {
		writeJSON(w, http.StatusOK, envelope{Code: CodeNoAvail, Message: "该商户暂无可认领设备", Data: data})
		return
	}
	writeOK(w, data)
}

// hBind 设备端绑定(认领/恢复,幂等兼心跳);成功返回密钥供客户端写本地 MQTT 配置。
func (s *Server) hBind(w http.ResponseWriter, r *http.Request) {
	var req struct {
		MerchantNo  string                  `json:"merchantNo"`
		DeviceName  string                  `json:"deviceName"`
		Fingerprint fingerprint.Fingerprint `json:"fingerprint"`
		DeviceInfo  *DeviceInfo             `json:"deviceInfo"`
	}
	if err := readJSON(r, &req); err != nil || req.MerchantNo == "" {
		writeErr(w, http.StatusOK, CodeBadParam, "参数错误")
		return
	}
	if req.DeviceName == "" {
		writeErr(w, http.StatusOK, CodeBadParam, "deviceName 不能为空")
		return
	}
	m, err := s.Store.FindMerchantByNo(req.MerchantNo)
	if err != nil {
		writeErr(w, http.StatusOK, CodeMerchantNone, "商户号不存在")
		return
	}
	if err := req.Fingerprint.Validate(); err != nil {
		writeErr(w, http.StatusOK, CodeFpInvalid, "设备指纹无效")
		return
	}
	fpHash := req.Fingerprint.Hash()
	rawJSON, _ := json.Marshal(req.Fingerprint)
	di := deviceInfoOf(&req.Fingerprint, req.DeviceInfo)

	d, err := s.Store.Bind(m.ID, req.DeviceName, fpHash, string(rawJSON), di.OsType, di.AppVersion)
	var be *store.BoundElseError
	switch {
	case err == nil:
		writeOK(w, map[string]interface{}{
			"deviceName":   d.Name,
			"deviceSecret": d.DeviceSecret,
			"productKey":   d.ProductKey,
			"status":       "bound",
		})
	case errors.As(err, &be):
		writeErr(w, http.StatusOK, CodeBoundElse, be.Error())
	case errors.Is(err, store.ErrNotOwned):
		writeErr(w, http.StatusOK, CodeNotOwned, err.Error())
	case errors.Is(err, store.ErrOccupied):
		writeErr(w, http.StatusOK, CodeOccupied, err.Error())
	default:
		writeErr(w, http.StatusServiceUnavailable, 5000, "服务内部错误")
	}
}
