// IotFetch.go 「获取设备信息」:采本机硬件指纹 → web 管理端 lookup(只查询,不绑定) →
// 回填 DeviceName/ProductKey/DeviceSecret 与设备源地址、商户号。
// 业务:先查本机是否已激活绑定(已绑=锁定恢复、不能切换;未绑=商户名下可认领任选)。
// 绑定确认在用户点「保存设置」后由 SettingsView.reportIotBind 调 /device/bind 上报(兼心跳)。
// 接口契约见 web/api/API.md 第 7 节组 E(POST /device/lookup、/device/bind,无 JWT)。
package ui

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"

	"congmingpay/internal/config"
	"congmingpay/internal/fingerprint"
)

// normalizeManageServer 规范化设备源地址:去空白、补 http:// 前缀、去尾部 /。
func normalizeManageServer(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if !strings.HasPrefix(s, "http://") && !strings.HasPrefix(s, "https://") {
		s = "http://" + s
	}
	return strings.TrimRight(s, "/")
}

// iotDeviceInfo 组装随指纹上送的展示信息(含版本号,web 设备页「平台/版本」来源)。
func iotDeviceInfo(fp fingerprint.Fingerprint) map[string]string {
	return map[string]string{
		"osType":     "win",
		"appVersion": config.AppVersion,
		"osBuild":    fp.OsBuild,
	}
}

// iotFingerprint 采集硬件指纹并填充展示字段(版本/系统版本)。
func iotFingerprint() (fingerprint.Fingerprint, error) {
	fp, err := fingerprint.Collect()
	if err != nil {
		return fp, err
	}
	fp.AppVersion = config.AppVersion
	fp.OsBuild = fingerprint.OsBuild()
	return fp, nil
}

type iotEnvelope struct {
	Code    int             `json:"code"`
	Message string          `json:"message"`
	Data    json.RawMessage `json:"data"`
}

func iotPost(url string, body []byte, out *iotEnvelope) error {
	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Post(url, "application/json; charset=utf-8", bytes.NewReader(body))
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

// iotDevice 是 lookup 返回的候选设备(名称+密钥,供回填本地 MQTT 配置)。
type iotDevice struct {
	Name         string `json:"name"`
	ProductKey   string `json:"productKey"`
	DeviceSecret string `json:"deviceSecret"`
}

// iotLookup 结果:OK=请求成功;Bound 非 nil=本机指纹已绑定(重装恢复);
// Available=该商户名下可认领设备;Msg=错误文案(code!=0 或网络失败)。
// lookup 只查询不绑定;绑定由「保存设置」后的 iotBind 上报确认。
type iotLookupResult struct {
	OK        bool
	Bound     *iotDevice
	Available []iotDevice
	Msg       string
}

func iotLookup(base, merchantNo string, fp fingerprint.Fingerprint) iotLookupResult {
	body, _ := json.Marshal(map[string]interface{}{
		"merchantNo": merchantNo,
		"fingerprint": fp,
		"deviceInfo":  iotDeviceInfo(fp),
	})
	var env iotEnvelope
	if err := iotPost(base+"/api/v1/device/lookup", body, &env); err != nil {
		return iotLookupResult{Msg: "无法连接设备源:" + err.Error()}
	}
	if env.Code != 0 {
		return iotLookupResult{Msg: env.Message}
	}
	var data struct {
		Merchant         map[string]string `json:"merchant"`
		Bound            *iotDevice        `json:"boundDevice"`
		AvailableDevices []iotDevice       `json:"availableDevices"`
	}
	_ = json.Unmarshal(env.Data, &data)
	return iotLookupResult{OK: true, Bound: data.Bound, Available: data.AvailableDevices}
}

// iotBind 结果:OK=绑定/恢复成功(返回密钥供写本地 MQTT 配置);Msg=错误文案。
type iotBindResult struct {
	OK           bool
	DeviceName   string
	ProductKey   string
	DeviceSecret string
	Msg          string
}

// iotBind 向管理端上报绑定确认(认领/恢复,幂等兼心跳);
// 由「保存设置」后的 onSaveSettings(reportIotBind)调用,不在对话框内调用。
func iotBind(base, merchantNo, deviceName string, fp fingerprint.Fingerprint) iotBindResult {
	body, _ := json.Marshal(map[string]interface{}{
		"merchantNo": merchantNo,
		"deviceName": deviceName,
		"fingerprint": fp,
		"deviceInfo":  iotDeviceInfo(fp),
	})
	var env iotEnvelope
	if err := iotPost(base+"/api/v1/device/bind", body, &env); err != nil {
		return iotBindResult{Msg: "无法连接设备源:" + err.Error()}
	}
	if env.Code != 0 {
		return iotBindResult{Msg: env.Message}
	}
	var data struct {
		DeviceName   string `json:"deviceName"`
		ProductKey   string `json:"productKey"`
		DeviceSecret string `json:"deviceSecret"`
	}
	_ = json.Unmarshal(env.Data, &data)
	return iotBindResult{OK: true, DeviceName: data.DeviceName, ProductKey: data.ProductKey, DeviceSecret: data.DeviceSecret}
}

// iotManageServerFixed 设备源地址暂固化(正式部署前改这里);对话框不再显示该字段。
const iotManageServerFixed = "http://127.0.0.1:9000"

// iotManageText 已保存值优先,空则用固化地址。
func iotManageText(saved string) string {
	if s := strings.TrimSpace(saved); s != "" {
		return s
	}
	return iotManageServerFixed
}

// runIotMerchantDialog 运行「获取设备信息」小窗:只输商户号,确定即开始查询。
// 返回 (商户号, 是否点确定)。商户号初值取本地已保存值;
// 值在 OnClicked 闭包里捕获(Accept 前),不在对话框关闭后读控件(关闭后读得空)。
func (a *App) runIotMerchantDialog() (string, bool) {
	var dlg *walk.Dialog
	var le *walk.LineEdit
	out, ok := "", false
	_, _ = (Dialog{
		AssignTo: &dlg,
		Title:    "获取设备信息",
		MinSize:  Size{Width: 360, Height: 140},
		Layout:   VBox{Spacing: 10},
		Children: []Widget{
			Label{Text: "商户号:"},
			LineEdit{
				AssignTo:  &le,
				Text:      strings.TrimSpace(a.iotMerchantNo.Text()),
				CueBanner: "长商户号或短商户号",
			},
			Composite{
				Layout: HBox{Spacing: 8},
				Children: []Widget{
					HSpacer{},
					PushButton{Text: "取消", OnClicked: func() { dlg.Cancel() }},
					PushButton{Text: "确定", OnClicked: func() {
						out = strings.TrimSpace(le.Text())
						ok = true
						dlg.Accept()
					}},
				},
			},
		},
	}).Run(a.mw)
	return out, ok
}

// applyIotLookup 处理 lookup 结果(仅 UI 线程;由 onIotFetchDevices 的查询 goroutine 回投):
// 已绑定 → 居中确认「该设备已绑定 X,是否直接使用?」;未绑定 → 可认领设备填进主界面 SN 下拉。
// 只查询不绑定;绑定确认在「保存设置」后由 reportIotBind 上报。
func (a *App) applyIotLookup(res iotLookupResult) {
	if res.Bound == nil && len(res.Available) == 0 {
		a.toast(toastError, res.Msg)
		return
	}
	// 固化地址写入隐藏行(保存时随配置持久化,reportIotBind 据此上报)
	if strings.TrimSpace(a.iotManageServer.Text()) == "" {
		_ = a.iotManageServer.SetText(iotManageServerFixed)
	}
	if res.Bound != nil {
		a.toastConfirm(fmt.Sprintf("该设备已绑定 %s，是否直接使用？", res.Bound.Name), func(yes bool) {
			if !yes {
				return
			}
			a.fillIotSN(res.Bound)
			a.iotSNBox.SetEnabled(false) // 已绑定锁定,不能切换
			a.onIotPreview()
			a.toast(toastInfo, fmt.Sprintf("已恢复绑定设备 %s,点「保存设置」完成恢复绑定", res.Bound.Name))
		})
		return
	}
	a.iotSnOpts = res.Available
	names := make([]string, 0, len(res.Available))
	for _, d := range res.Available {
		names = append(names, d.Name)
	}
	_ = a.iotSNBox.SetModel(names)
	_ = a.iotSNBox.SetCurrentIndex(0)
	a.iotSNBox.SetEnabled(true)
	a.onIotSNChanged() // 回填首项 DeviceName/PK/Secret 并刷新预览
	a.toast(toastSuccess, fmt.Sprintf("找到 %d 台可认领设备,请选择自由打印SN后点「保存设置」完成绑定", len(res.Available)))
}

// fillIotSN 把设备(名称+密钥)回填进主界面 SN 下拉与隐藏行。
func (a *App) fillIotSN(d *iotDevice) {
	a.iotSnOpts = []iotDevice{*d}
	_ = a.iotDeviceName.SetText(d.Name)
	_ = a.iotProductKey.SetText(d.ProductKey)
	_ = a.iotDeviceSecret.SetText(d.DeviceSecret)
	_ = a.iotSNBox.SetModel([]string{d.Name})
	_ = a.iotSNBox.SetCurrentIndex(0)
}

