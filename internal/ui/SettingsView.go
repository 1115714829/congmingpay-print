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

// 系统设置拆为两个顶层页(左导航「常规」「云端 MQTT」),800×600 下一屏排完、
// 无 ScrollView/嵌套 Tab——避免深层嵌套滚动在部分机器上切换卡顿,也保证直接填写并保存。

// generalPageWidget 常规页:服务名/系统通知/云盒兼容/历史保留 + 本地接口文档服务。
func (a *App) generalPageWidget() Composite {
	s := a.cfg.Settings
	return Composite{
		AssignTo: &a.generalPage,
		Visible:  false, // 构建即隐藏,避免叠加进窗口布局最小高度(切页时才显示)
		Layout:   VBox{Margins: Margins{Left: 16, Top: 10, Right: 16, Bottom: 8}, Spacing: 6},
		Children: []Widget{
			GroupBox{
				Title:  "服务",
				Layout: Grid{Columns: 2, Spacing: 6, Margins: Margins{Left: 10, Top: 8, Right: 10, Bottom: 8}},
				Children: []Widget{
					Label{Text: "服务名称:"},
					LineEdit{AssignTo: &a.setName, Text: s.ServiceName, CueBanner: "窗口标题与托盘提示用;空=默认"},
					Label{Text: "系统通知:"},
					CheckBox{AssignTo: &a.notifyEnabled, Text: "启用 Windows 系统通知(打印失败/卡单/打印机离线恢复/云端断线恢复)", Checked: !s.NotifyDisabled},
					Label{Text: "云盒兼容模式:"},
					CheckBox{AssignTo: &a.yunheCompat, Text: "启用云盒兼容模式(旧报文与精简回执)", Checked: s.YunheCompat()},
					Label{Text: ""},
					// 长说明 Label 默认 MinSize=整段文字宽(单行不折行),是本页最小宽的真正来源,
					// 会把所在列顶过 686 内容宽、切页时按 MinSizeForSize 撑大主窗口且不缩回。
					// 设 EllipsisMode:EllipsisEnd 后该项最小宽归 0、超宽省略号+悬停看全文;
					// 列最小宽改由「系统通知」勾选框(~430px,仍<686)决定,本页稳在预算内。
					Label{Text: "开启后接受云盒旧报文(type=5、参数可省、contents 切纸)且云端回执仅 done/failed;关闭后仅本服务标准协议与完整回执。", TextColor: colGray, EllipsisMode: EllipsisEnd},
					Label{Text: "打印历史保留天数:"},
					LineEdit{AssignTo: &a.jobHistoryDays, Text: strconv.Itoa(model.ClampJobHistoryDays(s.JobHistoryDays)), MaxSize: Size{Width: 120}, CueBanner: "1-365,默认7"},
					Label{Text: ""},
					Label{Text: "仅自动清理「已完成/失败」任务;排队中与等待重试不受限。落盘文件 data\\jobs.db。", TextColor: colGray, EllipsisMode: EllipsisEnd},
				},
			},
			GroupBox{
				Title:  "本地接口文档服务(仅局域网,只读)",
				Layout: Grid{Columns: 2, Spacing: 6, Margins: Margins{Left: 10, Top: 8, Right: 10, Bottom: 8}},
				Children: []Widget{
					Label{Text: "启用:"},
					CheckBox{AssignTo: &a.docEnabled, Text: "启动在线接口文档页(仅供查看,不参与数据通信)", Checked: s.DocServer.Enabled},
					Label{Text: "端口:"},
					LineEdit{AssignTo: &a.docPort, Text: strconv.Itoa(docPortOr(s.DocServer.Port)), MaxSize: Size{Width: 90}},
					Label{Text: "访问地址:"},
				Label{AssignTo: &a.docURL, Text: docURLText(s.DocServer), TextColor: colGray, EllipsisMode: EllipsisEnd},
			},
		},
		// 默认 Alignment 的 VBox 在高度富余时会整体垂直居中(首项前加 halfExcessShare),
		// 造成内容不贴顶的空位。这里放一个贪婪 VSpacer 吸收全部多余高度:
		// 上方 GroupBox 紧贴顶部,「保存设置」沉底,与打印机/队列页(表格贪婪吃满)一致。
		VSpacer{},
		Composite{
			Layout: HBox{Spacing: 8},
			Children: []Widget{
				HSpacer{},
				PushButton{Text: "保存设置", OnClicked: a.onSaveSettings},
			},
		},
	},
}
}

// mqttPageWidget 云端 MQTT 页:启用开关 + Provider 三选一(原生单选,直接切换下方表单)+ 三组表单与只读预览。
func (a *App) mqttPageWidget() Composite {
	s := a.cfg.Settings
	ali := s.MQTT.Aliyun
	if ali.Port <= 0 {
		ali.Port = 1883
	}
	iot := s.MQTT.Iot
	if iot.Port <= 0 {
		iot.Port = 1883
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
	iotHost, iotUser, iotCID := "—", "—", "(填写 ProductKey 与 DeviceName 后显示)"
	iotPrevSub, iotPrevRep := "—", "—"
	if h, u, id, ps, pr, err := mqtt.PreviewIot(iot); err != nil {
		iotHost = "(待补全: " + err.Error() + ")"
		if u != "" {
			iotUser = u
		}
		if id != "" {
			iotCID = id
		}
		iotPrevSub, iotPrevRep = orDash(ps), orDash(pr)
	} else {
		iotHost, iotUser, iotCID = orDash(h), orDash(u), cidOrDash(id)
		iotPrevSub, iotPrevRep = orDash(ps), orDash(pr)
	}
	return Composite{
		AssignTo: &a.mqttPage,
		Visible:  false, // 构建即隐藏,避免叠加进窗口布局最小高度(切页时才显示)
		Layout:   VBox{Margins: Margins{Left: 16, Top: 8, Right: 16, Bottom: 6}, Spacing: 5},
		Children: []Widget{
			Composite{
				Layout: HBox{Spacing: 12, MarginsZero: true},
				Children: []Widget{
					Label{Text: "启用:"},
					CheckBox{AssignTo: &a.mqttEnabled, Text: "启用 MQTT 连接", Checked: s.MQTT.Enabled},
					HSpacer{},
					Label{Text: "连接状态:"},
					// 动态状态/主题串可能很长,单行 Label 默认最小宽=整段文字宽,会把本页最小宽
					// 顶过 686 内容宽、切页撑大主窗口;EllipsisEnd 使其最小宽归 0、超宽省略号。
					Label{AssignTo: &a.mqttStatus, Text: "—", TextColor: colGray, EllipsisMode: EllipsisEnd},
				},
			},
			// 同容器相邻 RadioButton 自动组成互斥组(BS_AUTORADIOBUTTON);
			// 初始选中与切换回调由 initMqttProviderSelection 统一处理。
			Composite{
				Layout: HBox{Spacing: 16, Margins: Margins{Left: 2, Top: 0, Right: 0, Bottom: 0}},
				Children: []Widget{
					Label{Text: "接入方式:"},
					RadioButton{AssignTo: &a.mqttProvGeneric, Text: "自建 MQTT", OnClicked: func() { a.showMqttForm(model.MQTTProviderGeneric) }},
					RadioButton{AssignTo: &a.mqttProvAli, Text: "云消息队列", OnClicked: func() { a.showMqttForm(model.MQTTProviderAliyun) }},
					RadioButton{AssignTo: &a.mqttProvIot, Text: "物联网平台", OnClicked: func() { a.showMqttForm(model.MQTTProviderIot) }},
					HSpacer{},
				},
			},
			// 三组 Provider 表单同时构建、按选中显隐(无动态创建/销毁,避免嵌套重建卡顿)
			Composite{
				AssignTo: &a.mqttForms,
				Layout:   VBox{Margins: Margins{Left: 2, Top: 2, Right: 0, Bottom: 0}, Spacing: 2},
				Children: []Widget{
					a.mqttFormGeneric(),
					a.mqttFormAliyun(prevSub, prevRep, prevCID),
					a.mqttFormIot(iotPrevSub, iotPrevRep, iotHost, iotUser, iotCID),
			},
		},
		// 贪婪 VSpacer 吸收多余高度:上方表单紧贴顶部(默认 Alignment 的 VBox 会把非贪婪
		// 内容整体垂直居中,造成不贴顶的空位),按钮行沉底。
		VSpacer{},
		Composite{
			Layout: HBox{Spacing: 8},
			Children: []Widget{
				PushButton{Text: "连接测试", OnClicked: a.onTestMQTT},
					HSpacer{},
					PushButton{Text: "保存设置", OnClicked: a.onSaveSettings},
				},
			},
		},
	}
}

// mqttFormGeneric 自建 MQTT 表单(6 字段,紧凑 2 列)。
func (a *App) mqttFormGeneric() GroupBox {
	s := a.cfg.Settings
	return GroupBox{
		Title:  "自建 MQTT(用户名密码)",
		Layout: Grid{Columns: 2, Spacing: 5, Margins: Margins{Left: 8, Top: 6, Right: 8, Bottom: 6}},
		Children: []Widget{
			Label{Text: "Broker:"},
			LineEdit{AssignTo: &a.mqttBroker, Text: s.MQTT.Broker, CueBanner: "mqtt.example.com 或 tcp://host:1883"},
			Label{Text: "端口:"},
			LineEdit{AssignTo: &a.mqttPort, Text: strconv.Itoa(portOr(s.MQTT.Port, 1883)), MaxSize: Size{Width: 90}},
			Label{Text: "用户名:"},
			LineEdit{AssignTo: &a.mqttUser, Text: s.MQTT.Username},
			Label{Text: "密码:"},
			LineEdit{AssignTo: &a.mqttPass, Text: s.MQTT.Password, PasswordMode: true},
			Label{Text: "短商户号:"},
			LineEdit{AssignTo: &a.mqttTopic, Text: s.MQTT.Topic, CueBanner: "订阅主题;仅字母数字下划线", OnTextChanged: a.onMerchantChanged},
			Label{Text: "上报主题:"},
			LineEdit{AssignTo: &a.mqttReport, Text: s.MQTT.ReportTopic, CueBanner: "上行发布主题,必填,如 server"},
		},
	}
}

// mqttFormAliyun 云消息队列 MQTT 表单(8 字段 + 端口,紧凑 2 列;3 个预览合成 1 行)。
func (a *App) mqttFormAliyun(prevSub, prevRep, prevCID string) GroupBox {
	ali := a.cfg.Settings.MQTT.Aliyun
	if ali.Port <= 0 {
		ali.Port = 1883
	}
	return GroupBox{
		Title:  "云消息队列 MQTT 版(阿里云 AK/SK)",
		Layout: Grid{Columns: 2, Spacing: 5, Margins: Margins{Left: 8, Top: 6, Right: 8, Bottom: 6}},
		Children: []Widget{
			Label{Text: "MQTT 地址:"},
			LineEdit{AssignTo: &a.aliEndpoint, Text: ali.Endpoint, CueBanner: "xxxx.mqtt.aliyuncs.com"},
			Label{Text: "端口:"},
			LineEdit{AssignTo: &a.aliPort, Text: strconv.Itoa(portOr(ali.Port, 1883)), MaxSize: Size{Width: 90}},
			Label{Text: "实例 ID:"},
			LineEdit{AssignTo: &a.aliInstance, Text: ali.InstanceId, CueBanner: "实例 ID"},
			Label{Text: "AccessKey:"},
			LineEdit{AssignTo: &a.aliAccessKey, Text: ali.AccessKey},
			Label{Text: "SecretKey:"},
			LineEdit{AssignTo: &a.aliSecretKey, Text: ali.SecretKey, PasswordMode: true},
			Label{Text: "GroupId:"},
			LineEdit{AssignTo: &a.aliGroupId, Text: ali.GroupId, CueBanner: "GID_xxx", OnTextChanged: a.onAliyunPreview},
			Label{Text: "父主题:"},
			LineEdit{AssignTo: &a.aliParent, Text: ali.ParentTopic, CueBanner: "如 server", OnTextChanged: a.onAliyunPreview},
			Label{Text: "自定义ID:"},
			LineEdit{AssignTo: &a.aliDeviceId, Text: ali.DeviceId, CueBanner: "商户号/自定义ID", OnTextChanged: a.onAliyunPreview},
			Label{Text: "下行后缀:"},
			LineEdit{AssignTo: &a.aliDownSuffix, Text: ali.DownSuffix, CueBanner: "cmd", OnTextChanged: a.onAliyunPreview},
			Label{Text: "上行后缀:"},
			LineEdit{AssignTo: &a.aliUpSuffix, Text: ali.UpSuffix, CueBanner: "report", OnTextChanged: a.onAliyunPreview},
			Label{Text: "实际订阅:"},
			Label{AssignTo: &a.aliPreviewSub, Text: prevSub, TextColor: colGray, EllipsisMode: EllipsisEnd},
			Label{Text: "实际上报 / ClientID:"},
			Composite{
				Layout: HBox{Spacing: 12, MarginsZero: true},
				Children: []Widget{
					Label{AssignTo: &a.aliPreviewRep, Text: orDash(prevRep), TextColor: colGray, EllipsisMode: EllipsisEnd},
					Label{AssignTo: &a.aliPreviewCID, Text: cidOrDash(prevCID), TextColor: colGray, EllipsisMode: EllipsisEnd},
				},
			},
		},
	}
}

// mqttFormIot 物联网平台表单(8 输入行,紧凑 2 列;5 个预览占 2 行)。
func (a *App) mqttFormIot(prevSub, prevRep, prevHost, prevUser, prevCID string) GroupBox {
	iot := a.cfg.Settings.MQTT.Iot
	if iot.Port <= 0 {
		iot.Port = 1883
	}
	return GroupBox{
		Title:  "物联网平台(一机一密)",
		Layout: Grid{Columns: 2, Spacing: 5, Margins: Margins{Left: 8, Top: 6, Right: 8, Bottom: 6}},
		Children: []Widget{
			Label{Text: "ProductKey:"},
			LineEdit{AssignTo: &a.iotProductKey, Text: iot.ProductKey, OnTextChanged: a.onIotPreview},
			Label{Text: "DeviceName:"},
			Composite{
				Layout: HBox{Spacing: 6, MarginsZero: true},
				Children: []Widget{
					LineEdit{AssignTo: &a.iotDeviceName, Text: iot.DeviceName, CueBanner: "设备名称", OnTextChanged: a.onIotPreview},
					PushButton{Text: "设备列表", MaxSize: Size{Width: 84}, OnClicked: a.onIotFetchDevices},
				},
			},
			Label{Text: "DeviceSecret:"},
			LineEdit{AssignTo: &a.iotDeviceSecret, Text: iot.DeviceSecret, PasswordMode: true},
			Label{Text: "地域(可选):"},
			LineEdit{AssignTo: &a.iotRegionId, Text: iot.RegionId, CueBanner: "未填 MQTT 地址时拼接入点,如 cn-shanghai", OnTextChanged: a.onIotPreview},
			Label{Text: "MQTT 地址(可选):"},
			LineEdit{AssignTo: &a.iotEndpoint, Text: iot.Endpoint, CueBanner: "控制台接入点;空则按 ProductKey+地域拼接", OnTextChanged: a.onIotPreview},
			Label{Text: "端口:"},
			LineEdit{AssignTo: &a.iotPort, Text: strconv.Itoa(portOr(iot.Port, 1883)), MaxSize: Size{Width: 90}},
			Label{Text: "下行后缀:"},
			LineEdit{AssignTo: &a.iotDownSuffix, Text: iot.DownSuffix, CueBanner: "cmd → /pk/dn/user/cmd", OnTextChanged: a.onIotPreview},
			Label{Text: "上行后缀:"},
			LineEdit{AssignTo: &a.iotUpSuffix, Text: iot.UpSuffix, CueBanner: "report → /pk/dn/user/report", OnTextChanged: a.onIotPreview},
			Label{Text: "订阅 / 上报:"},
			Composite{
				Layout: HBox{Spacing: 12, MarginsZero: true},
				Children: []Widget{
					Label{AssignTo: &a.iotPreviewSub, Text: prevSub, TextColor: colGray, EllipsisMode: EllipsisEnd},
					Label{AssignTo: &a.iotPreviewRep, Text: prevRep, TextColor: colGray, EllipsisMode: EllipsisEnd},
				},
			},
			Label{Text: "接入点 / User / CID:"},
			Composite{
				Layout: HBox{Spacing: 12, MarginsZero: true},
				Children: []Widget{
					Label{AssignTo: &a.iotPreviewHost, Text: prevHost, TextColor: colGray, EllipsisMode: EllipsisEnd},
					Label{AssignTo: &a.iotPreviewUser, Text: orDash(prevUser), TextColor: colGray, EllipsisMode: EllipsisEnd},
					Label{AssignTo: &a.iotPreviewCID, Text: cidOrDash(prevCID), TextColor: colGray, EllipsisMode: EllipsisEnd},
				},
			},
		},
	}
}

// showMqttForm 按 Provider 显隐三组表单(同页切换,不重建控件;子项顺序:自建/云消息队列/物联网)。
func (a *App) showMqttForm(prov string) {
	if a.mqttForms == nil {
		return
	}
	forms := a.mqttForms.Children()
	if forms.Len() != 3 {
		return
	}
	forms.At(0).SetVisible(prov == model.MQTTProviderGeneric)
	forms.At(1).SetVisible(prov == model.MQTTProviderAliyun)
	forms.At(2).SetVisible(prov == model.MQTTProviderIot)
}

// initMqttProviderSelection 按已保存 Provider 初始选中对应单选,并按选中显隐三组表单。
// 须在主窗口 Create 之后调用(单选控件此时已建成)。
func (a *App) initMqttProviderSelection() {
	if a.mqttProvGeneric == nil || a.mqttProvAli == nil || a.mqttProvIot == nil {
		return
	}
	switch a.cfg.Settings.MQTT.EffectiveProvider() {
	case model.MQTTProviderAliyun:
		a.mqttProvAli.SetChecked(true)
	case model.MQTTProviderIot:
		a.mqttProvIot.SetChecked(true)
	default:
		a.mqttProvGeneric.SetChecked(true)
	}
	a.showMqttForm(a.currentProvider())
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
		return "(填写后显示)"
	}
	return s
}

func (a *App) currentProvider() string {
	if a.mqttProvAli != nil && a.mqttProvAli.Checked() {
		return model.MQTTProviderAliyun
	}
	if a.mqttProvIot != nil && a.mqttProvIot.Checked() {
		return model.MQTTProviderIot
	}
	return model.MQTTProviderGeneric
}

func (a *App) currentAliyun() model.AliyunMQTT {
	return model.AliyunMQTT{
		Endpoint:    strings.TrimSpace(a.aliEndpoint.Text()),
		Port:        atoiOr(a.aliPort.Text(), 1883),
		InstanceId:  strings.TrimSpace(a.aliInstance.Text()),
		AccessKey:   strings.TrimSpace(a.aliAccessKey.Text()),
		SecretKey:   strings.TrimSpace(a.aliSecretKey.Text()),
		GroupId:     strings.TrimSpace(a.aliGroupId.Text()),
		ParentTopic: strings.TrimSpace(a.aliParent.Text()),
		DeviceId:    strings.TrimSpace(a.aliDeviceId.Text()),
		DownSuffix:  strings.TrimSpace(a.aliDownSuffix.Text()),
		UpSuffix:    strings.TrimSpace(a.aliUpSuffix.Text()),
	}
}

func (a *App) currentIot() model.IoTMQTT {
	return model.IoTMQTT{
		ProductKey:   strings.TrimSpace(a.iotProductKey.Text()),
		DeviceName:   strings.TrimSpace(a.iotDeviceName.Text()),
		DeviceSecret: strings.TrimSpace(a.iotDeviceSecret.Text()),
		RegionId:     strings.TrimSpace(a.iotRegionId.Text()),
		Endpoint:     strings.TrimSpace(a.iotEndpoint.Text()),
		Port:         atoiOr(a.iotPort.Text(), 1883),
		DownSuffix:   strings.TrimSpace(a.iotDownSuffix.Text()),
		UpSuffix:     strings.TrimSpace(a.iotUpSuffix.Text()),
	}
}

func (a *App) onIotPreview() {
	if a.iotPreviewHost == nil {
		return
	}
	host, user, cid, sub, rep, err := mqtt.PreviewIot(a.currentIot())
	if err != nil {
		_ = a.iotPreviewHost.SetText("(待补全: " + err.Error() + ")")
		_ = a.iotPreviewUser.SetText(orDash(user))
		_ = a.iotPreviewCID.SetText(cidOrDash(cid))
		_ = a.iotPreviewSub.SetText(orDash(sub))
		_ = a.iotPreviewRep.SetText(orDash(rep))
		return
	}
	_ = a.iotPreviewHost.SetText(orDash(host))
	_ = a.iotPreviewUser.SetText(orDash(user))
	_ = a.iotPreviewCID.SetText(cidOrDash(cid))
	_ = a.iotPreviewSub.SetText(orDash(sub))
	_ = a.iotPreviewRep.SetText(orDash(rep))
}

// onIotFetchDevices 设备列表拉取占位(后续接物联网 OpenAPI)。
func (a *App) onIotFetchDevices() {
	a.warn("功能预留", "获取设备列表尚未实现。\r\n请暂时手动填写 DeviceName、ProductKey、DeviceSecret。")
}

func (a *App) currentMQTT() model.MQTT {
	return model.MQTT{
		Enabled:     a.mqttEnabled.Checked(),
		Provider:    a.currentProvider(),
		Broker:      strings.TrimSpace(a.mqttBroker.Text()),
		Port:        atoiOr(a.mqttPort.Text(), 1883),
		Username:    strings.TrimSpace(a.mqttUser.Text()),
		Password:    strings.TrimSpace(a.mqttPass.Text()),
		Topic:       mqtt.SanitizeMerchant(a.mqttTopic.Text()),
		ReportTopic: strings.TrimSpace(a.mqttReport.Text()),
		Aliyun:      a.currentAliyun(),
		Iot:         a.currentIot(),
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
	s.YunheCompatDisabled = !a.yunheCompat.Checked()
	s.JobHistoryDays = model.ClampJobHistoryDays(atoiOr(a.jobHistoryDays.Text(), model.DefaultJobHistoryDays))
	s.MQTT = m

	s.DocServer.Enabled = a.docEnabled.Checked()
	s.DocServer.Port = docPortOr(atoiOr(a.docPort.Text(), docserver.DefaultPort))

	a.save()
	a.flash("设置已保存")
	if a.svc != nil {
		a.svc.SetHistoryDays(s.JobHistoryDays)
	}
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
		switch m.EffectiveProvider() {
		case model.MQTTProviderAliyun:
			return fmt.Errorf("启用云消息队列时请补全:地址、端口、InstanceId、AccessKey、SecretKey、GroupId、父主题、自定义 ID、下行后缀、上行后缀")
		case model.MQTTProviderIot:
			return fmt.Errorf("启用物联网平台时请补全:ProductKey、DeviceName、DeviceSecret、下行后缀、上行后缀；并填写 MQTT 地址，或填写地域以自动拼接入点")
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
		if strings.TrimSpace(m.Password) == "" {
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
	switch m.EffectiveProvider() {
	case model.MQTTProviderGeneric:
		if strings.TrimSpace(m.Username) == "" || strings.TrimSpace(m.Password) == "" {
			return fmt.Errorf("启用自建 MQTT 时用户名与密码均为必填")
		}
		if m.Port <= 0 || m.Port > 65535 {
			return fmt.Errorf("端口须为 1-65535")
		}
	case model.MQTTProviderAliyun:
		if m.Aliyun.Port <= 0 || m.Aliyun.Port > 65535 {
			return fmt.Errorf("端口须为 1-65535")
		}
	case model.MQTTProviderIot:
		if m.Iot.Port <= 0 || m.Iot.Port > 65535 {
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
