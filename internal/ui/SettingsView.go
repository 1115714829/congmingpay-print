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
				},
			},
			GroupBox{
				Title:  "云端 MQTT(唯一云端通道)",
				Layout: Grid{Columns: 2, Spacing: 8, Margins: Margins{Left: 12, Top: 10, Right: 12, Bottom: 10}},
				Children: []Widget{
					Label{Text: "启用:"},
					CheckBox{AssignTo: &a.mqttEnabled, Text: "启用 MQTT 连接", Checked: s.MQTT.Enabled},
					Label{Text: "Broker:"},
					LineEdit{AssignTo: &a.mqttBroker, Text: s.MQTT.Broker, CueBanner: "mqtt.example.com 或 tcp://host:1883"},
					Label{Text: "端口:"},
					LineEdit{AssignTo: &a.mqttPort, Text: strconv.Itoa(s.MQTT.Port)},
					Label{Text: "用户名:"},
					LineEdit{AssignTo: &a.mqttUser, Text: s.MQTT.Username},
					Label{Text: "密码:"},
					LineEdit{AssignTo: &a.mqttPass, Text: s.MQTT.Password, PasswordMode: true},
					Label{Text: "聪明付短商户号:"},
					LineEdit{AssignTo: &a.mqttTopic, Text: s.MQTT.Topic, CueBanner: "订阅主题;仅字母数字下划线", OnTextChanged: a.onMerchantChanged},
					Label{Text: "连接状态:"},
					Label{AssignTo: &a.mqttStatus, Text: "—", TextColor: colGray},
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

// onMerchantChanged 短商户号输入即净化(去空格/Tab/换行、仅留字母数字下划线)。
func (a *App) onMerchantChanged() {
	t := a.mqttTopic.Text()
	c := mqtt.SanitizeMerchant(t)
	if c != t {
		_ = a.mqttTopic.SetText(c) // 已净化后再次触发时 c==t,不再递归
	}
}

// currentMQTT 从设置页当前输入(未保存)组装 MQTT 参数。
func (a *App) currentMQTT() model.MQTT {
	return model.MQTT{
		Enabled:  a.mqttEnabled.Checked(),
		Broker:   strings.TrimSpace(a.mqttBroker.Text()),
		Port:     atoiOr(a.mqttPort.Text(), 1883),
		Username: a.mqttUser.Text(),
		Password: a.mqttPass.Text(),
		Topic:    mqtt.SanitizeMerchant(a.mqttTopic.Text()),
	}
}

// onTestMQTT 用当前输入做一次连接测试。
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
	s := &a.cfg.Settings
	s.ServiceName = strings.TrimSpace(a.setName.Text())
	s.MQTT.Enabled = a.mqttEnabled.Checked()
	s.MQTT.Broker = strings.TrimSpace(a.mqttBroker.Text())
	s.MQTT.Port = atoiOr(a.mqttPort.Text(), s.MQTT.Port)
	s.MQTT.Username = a.mqttUser.Text()
	s.MQTT.Password = a.mqttPass.Text()
	s.MQTT.Topic = mqtt.SanitizeMerchant(a.mqttTopic.Text())

	s.DocServer.Enabled = a.docEnabled.Checked()
	s.DocServer.Port = docPortOr(atoiOr(a.docPort.Text(), docserver.DefaultPort))

	a.save()
	a.flash("设置已保存")
	if a.mc != nil {
		a.mc.Reload(s.MQTT) // 按新配置重连
		a.refreshMqttStatus()
	}
	if a.ds != nil {
		a.ds.Reload(s.DocServer) // 按新配置重启接口文档服务
		if a.docURL != nil {
			_ = a.docURL.SetText(docURLText(s.DocServer))
		}
	}
}

// docPortOr 端口非法(<=0)时回落默认。
func docPortOr(p int) int {
	if p <= 0 {
		return docserver.DefaultPort
	}
	return p
}

// docURLText 返回接口文档服务的局域网访问地址(未启用则提示)。
func docURLText(cfg model.DocServer) string {
	if !cfg.Enabled {
		return "(未启用)"
	}
	return fmt.Sprintf("http://%s:%d/", docserver.LANIP(), docPortOr(cfg.Port))
}

// refreshMqttStatus 刷新连接状态标签。
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
