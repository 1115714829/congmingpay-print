package ui

import (
	"fmt"
	"strconv"
	"strings"

	"congmingpay/internal/docserver"
	"congmingpay/internal/model"
	"congmingpay/internal/mqtt"

	"github.com/lxn/walk"
	. "github.com/lxn/walk/declarative"
)

func (a *App) settingsPageWidget() Composite {
	s := a.cfg.Settings
	ali := s.MQTT.Aliyun
	if ali.Port <= 0 {
		ali.Port = 1883
	}
	prevSub, prevRep, prevCID := "—", "—", "(填写 GroupId 与自定义 ID 后显示)"
	if ps, pr, pc, err := mqtt.PreviewTopics(ali); err != nil {
		prevSub = "(待补全: " + err.Error() + ")"
		if pc != "" {
			prevCID = pc
		}
	} else {
		prevSub, prevRep = orDash(ps), orDash(pr)
		if pc != "" {
			prevCID = pc
		}
	}
	aliTab := 0
	if s.MQTT.EffectiveProvider() == model.MQTTProviderAliyun {
		aliTab = 1
	}
	a.mqttAliyunTab = aliTab
	return Composite{
		AssignTo: &a.settingsPage,
		Layout:   VBox{Margins: Margins{Left: 20, Top: 16, Right: 20, Bottom: 16}, Spacing: 10},
		Children: []Widget{
			Label{Text: "系统设置", Font: Font{PointSize: 12, Bold: true}},
			Label{Text: "打印服务的全局参数,点「保存设置」生效。", TextColor: colGray},
			GroupBox{
				Title:  "服务",
				Layout: Grid{Columns: 2, Spacing: 8, Margins: Margins{Left: 12, Top: 10, Right: 12, Bottom: 10}},
				Children: []Widget{
					Label{Text: "服务名称:"},
					LineEdit{AssignTo: &a.setName, Text: s.ServiceName},
					Label{Text: "系统通知:"},
					CheckBox{AssignTo: &a.notifyEnabled, Text: "启用 Windows 系统通知(打印失败/卡单/打印机离线恢复/云端断线恢复)", Checked: !s.NotifyDisabled},
				},
			},
			GroupBox{
				Title:  "云端 MQTT(唯一云端通道)",
				Layout: VBox{Margins: Margins{Left: 12, Top: 10, Right: 12, Bottom: 10}, Spacing: 8},
				Children: []Widget{
					Composite{
						Layout: HBox{Spacing: 8},
						Children: []Widget{
							Label{Text: "启用:"},
							CheckBox{AssignTo: &a.mqttEnabled, Text: "启用 MQTT 连接", Checked: s.MQTT.Enabled},
							HSpacer{},
							Label{Text: "连接状态:"},
							Label{AssignTo: &a.mqttStatus, Text: "—", TextColor: colGray},
						},
					},
					TabWidget{
						AssignTo: &a.mqttTabs,
						Pages: []TabPage{
							{
								Title:  "自建 MQTT",
								Layout: Grid{Columns: 2, Spacing: 8},
								Children: []Widget{
									Label{Text: "Broker:"},
									LineEdit{AssignTo: &a.mqttBroker, Text: s.MQTT.Broker, CueBanner: "mqtt.example.com 或 tcp://host:1883"},
									Label{Text: "端口:"},
									LineEdit{AssignTo: &a.mqttPort, Text: strconv.Itoa(portOr(s.MQTT.Port, 1883))},
									Label{Text: "用户名:"},
									LineEdit{AssignTo: &a.mqttUser, Text: s.MQTT.Username},
									Label{Text: "密码:"},
									LineEdit{AssignTo: &a.mqttPass, Text: s.MQTT.Password, PasswordMode: true},
									Label{Text: "聪明付短商户号:"},
									LineEdit{AssignTo: &a.mqttTopic, Text: s.MQTT.Topic, CueBanner: "订阅主题;仅字母数字下划线", OnTextChanged: a.onMerchantChanged},
									Label{Text: "上报主题:"},
									LineEdit{AssignTo: &a.mqttReport, Text: s.MQTT.ReportTopic, CueBanner: "上行发布主题,必填,如 server"},
								},
							},
							{
								Title:  "阿里云 MQTT",
								Layout: Grid{Columns: 2, Spacing: 8},
								Children: []Widget{
									Label{Text: "阿里云 MQTT 地址:"},
									LineEdit{AssignTo: &a.aliEndpoint, Text: ali.Endpoint, CueBanner: "xxxx.mqtt.aliyuncs.com"},
									Label{Text: "MQTT 端口:"},
									LineEdit{AssignTo: &a.aliPort, Text: strconv.Itoa(portOr(ali.Port, 1883))},
									Label{Text: "InstanceId:"},
									LineEdit{AssignTo: &a.aliInstance, Text: ali.InstanceId, CueBanner: "实例 ID"},
									Label{Text: "AccessKey:"},
									LineEdit{AssignTo: &a.aliAccessKey, Text: ali.AccessKey},
									Label{Text: "SecretKey:"},
									LineEdit{AssignTo: &a.aliSecretKey, Text: ali.SecretKey, PasswordMode: true},
									Label{Text: "GroupId:"},
									LineEdit{AssignTo: &a.aliGroupId, Text: ali.GroupId, CueBanner: "GID_xxx", OnTextChanged: a.onAliyunPreview},
									Label{Text: "父主题:"},
									LineEdit{AssignTo: &a.aliParent, Text: ali.ParentTopic, CueBanner: "如 server", OnTextChanged: a.onAliyunPreview},
									Label{Text: "自定义 ID(商户号):"},
									LineEdit{AssignTo: &a.aliDeviceId, Text: ali.DeviceId, OnTextChanged: a.onAliyunPreview},
									Label{Text: "下行后缀:"},
									LineEdit{AssignTo: &a.aliDownSuffix, Text: ali.DownSuffix, CueBanner: "cmd", OnTextChanged: a.onAliyunPreview},
									Label{Text: "上行后缀:"},
									LineEdit{AssignTo: &a.aliUpSuffix, Text: ali.UpSuffix, CueBanner: "report", OnTextChanged: a.onAliyunPreview},
									Label{Text: "实际订阅:"},
									Label{AssignTo: &a.aliPreviewSub, Text: prevSub, TextColor: colGray},
									Label{Text: "实际上报:"},
									Label{AssignTo: &a.aliPreviewRep, Text: prevRep, TextColor: colGray},
									Label{Text: "ClientID:"},
									Label{AssignTo: &a.aliPreviewCID, Text: prevCID, TextColor: colGray},
								},
							},
						},
					},
				},
			},
			GroupBox{
				Title:  "本地接口文档服务(仅局域网,只读)",
				Layout: Grid{Columns: 2, Spacing: 8, Margins: Margins{Left: 12, Top: 10, Right: 12, Bottom: 10}},
				Children: []Widget{
					Label{Text: "启用:"},
					CheckBox{AssignTo: &a.docEnabled, Text: "启动在线接口文档页(仅供查看,不参与数据通信)", Checked: s.DocServer.Enabled},
					Label{Text: "端口:"},
					LineEdit{AssignTo: &a.docPort, Text: strconv.Itoa(docPortOr(s.DocServer.Port)), MaxSize: Size{Width: 90}},
					Label{Text: "访问地址:"},
					Label{AssignTo: &a.docURL, Text: docURLText(s.DocServer), TextColor: colGray},
				},
			},
			Composite{
				Layout: HBox{Spacing: 8},
				Children: []Widget{
					PushButton{Text: "连接测试", OnClicked: a.onTestMQTT},
					HSpacer{},
					PushButton{Text: "保存设置", OnClicked: a.onSaveSettings},
				},
			},
			VSpacer{},
		},
	}
}

func (a *App) onMerchantChanged() {
	t := a.mqttTopic.Text()
	c := mqtt.SanitizeMerchant(t)
	if c != t {
		_ = a.mqttTopic.SetText(c)
	}
}

func (a *App) onAliyunPreview() {
	if a.aliPreviewSub == nil {
		return
	}
	sub, rep, cid, err := mqtt.PreviewTopics(a.currentAliyun())
	if err != nil {
		_ = a.aliPreviewSub.SetText("(待补全: " + err.Error() + ")")
		_ = a.aliPreviewRep.SetText("—")
		_ = a.aliPreviewCID.SetText(cidOrDash(cid))
		return
	}
	_ = a.aliPreviewSub.SetText(orDash(sub))
	_ = a.aliPreviewRep.SetText(orDash(rep))
	_ = a.aliPreviewCID.SetText(cidOrDash(cid))
}

func orDash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}

func cidOrDash(s string) string {
	if s == "" {
		return "(填写 GroupId 与自定义 ID 后显示)"
	}
	return s
}

func (a *App) currentProvider() string {
	if a.mqttTabs != nil && a.mqttTabs.CurrentIndex() == 1 {
		return model.MQTTProviderAliyun
	}
	return model.MQTTProviderGeneric
}

func (a *App) currentAliyun() model.AliyunMQTT {
	return model.AliyunMQTT{
		Endpoint:    strings.TrimSpace(a.aliEndpoint.Text()),
		Port:        atoiOr(a.aliPort.Text(), 1883),
		InstanceId:  strings.TrimSpace(a.aliInstance.Text()),
		AccessKey:   strings.TrimSpace(a.aliAccessKey.Text()),
		SecretKey:   a.aliSecretKey.Text(),
		GroupId:     strings.TrimSpace(a.aliGroupId.Text()),
		ParentTopic: strings.TrimSpace(a.aliParent.Text()),
		DeviceId:    strings.TrimSpace(a.aliDeviceId.Text()),
		DownSuffix:  strings.TrimSpace(a.aliDownSuffix.Text()),
		UpSuffix:    strings.TrimSpace(a.aliUpSuffix.Text()),
	}
}

func (a *App) currentMQTT() model.MQTT {
	return model.MQTT{
		Enabled:     a.mqttEnabled.Checked(),
		Provider:    a.currentProvider(),
		Broker:      strings.TrimSpace(a.mqttBroker.Text()),
		Port:        atoiOr(a.mqttPort.Text(), 1883),
		Username:    a.mqttUser.Text(),
		Password:    a.mqttPass.Text(),
		Topic:       mqtt.SanitizeMerchant(a.mqttTopic.Text()),
		ReportTopic: strings.TrimSpace(a.mqttReport.Text()),
		Aliyun:      a.currentAliyun(),
	}
}

func (a *App) onTestMQTT() {
	m := a.currentMQTT()
	a.setMqttStatus("连接测试中…", colGray)
	go func() {
		d, err := mqtt.TestConnect(m)
		a.mw.Synchronize(func() {
			if err != nil {
				a.setMqttStatus("连接失败: "+err.Error(), colRed)
			} else {
				a.setMqttStatus(fmt.Sprintf("连接成功(%dms)", d.Milliseconds()), colGreen)
			}
		})
	}()
}

func (a *App) setMqttStatus(text string, color walk.Color) {
	if a.mqttStatus != nil {
		_ = a.mqttStatus.SetText(text)
		a.mqttStatus.SetTextColor(color)
	}
}

func (a *App) onSaveSettings() {
	m := a.currentMQTT()
	if m.Enabled {
		if err := validateMQTTForSave(m); err != nil {
			a.warn("无法保存", err.Error())
			return
		}
	}

	s := &a.cfg.Settings
	s.ServiceName = strings.TrimSpace(a.setName.Text())
	s.NotifyDisabled = !a.notifyEnabled.Checked()
	s.MQTT = m

	s.DocServer.Enabled = a.docEnabled.Checked()
	s.DocServer.Port = docPortOr(atoiOr(a.docPort.Text(), docserver.DefaultPort))

	a.save()
	a.flash("设置已保存")
	if a.mw != nil {
		_ = a.mw.SetTitle(a.windowTitle())
	}
	if a.tray != nil {
		_ = a.tray.SetToolTip(a.serviceName())
	}
	if a.mc != nil {
		a.mc.Reload(s.MQTT)
		a.refreshMqttStatus()
	}
	if a.ds != nil {
		a.ds.Reload(s.DocServer)
		if a.docURL != nil {
			_ = a.docURL.SetText(docURLText(s.DocServer))
		}
	}
}

func validateMQTTForSave(m model.MQTT) error {
	_, ok, err := mqtt.Resolve(m)
	if err != nil {
		return err
	}
	if !ok {
		if m.EffectiveProvider() == model.MQTTProviderAliyun {
			return fmt.Errorf("启用阿里云 MQTT 时请补全:地址、端口、InstanceId、AccessKey、SecretKey、GroupId、父主题、自定义 ID、下行后缀、上行后缀")
		}
		var miss []string
		if strings.TrimSpace(m.Broker) == "" {
			miss = append(miss, "Broker")
		}
		if m.Port <= 0 || m.Port > 65535 {
			miss = append(miss, "端口(1-65535)")
		}
		if strings.TrimSpace(m.Username) == "" {
			miss = append(miss, "用户名")
		}
		if m.Password == "" {
			miss = append(miss, "密码")
		}
		if mqtt.SanitizeMerchant(m.Topic) == "" {
			miss = append(miss, "聪明付短商户号")
		}
		if strings.TrimSpace(m.ReportTopic) == "" {
			miss = append(miss, "上报主题")
		}
		if len(miss) > 0 {
			return fmt.Errorf("启用自建 MQTT 时以下选项均为必填:\r\n%s", strings.Join(miss, "、"))
		}
		return fmt.Errorf("MQTT 参数不完整")
	}
	if m.EffectiveProvider() == model.MQTTProviderGeneric {
		if strings.TrimSpace(m.Username) == "" || m.Password == "" {
			return fmt.Errorf("启用自建 MQTT 时用户名与密码均为必填")
		}
		if m.Port <= 0 || m.Port > 65535 {
			return fmt.Errorf("端口须为 1-65535")
		}
	}
	if m.EffectiveProvider() == model.MQTTProviderAliyun {
		if m.Aliyun.Port <= 0 || m.Aliyun.Port > 65535 {
			return fmt.Errorf("端口须为 1-65535")
		}
	}
	return nil
}

func docPortOr(p int) int {
	if p <= 0 {
		return docserver.DefaultPort
	}
	return p
}

func portOr(p, def int) int {
	if p <= 0 {
		return def
	}
	return p
}

func docURLText(cfg model.DocServer) string {
	if !cfg.Enabled {
		return "(未启用)"
	}
	return fmt.Sprintf("http://%s:%d/", docserver.LANIP(), docPortOr(cfg.Port))
}

func (a *App) refreshMqttStatus() {
	if a.mc == nil {
		return
	}
	ok, detail := a.mc.Status()
	if ok {
		a.setMqttStatus(detail, colGreen)
	} else {
		a.setMqttStatus(detail, colGray)
	}
}

func atoiOr(s string, def int) int {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return def
	}
	return n
}
