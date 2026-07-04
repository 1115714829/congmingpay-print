package ui

import (
	"strconv"
	"strings"

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
					Label{Text: "监听端口 (RAW/9100):"},
					LineEdit{AssignTo: &a.setPort, Text: s.ListenPort},
					Label{Text: "API 端口 (改后需重启):"},
					LineEdit{AssignTo: &a.setAPIPort, Text: s.APIPort},
					Label{Text: "默认纸张规格:"},
					ComboBox{AssignTo: &a.setPaperCB, MinSize: Size{Width: 90}, MaxSize: Size{Width: 90}, Model: []string{"58mm", "80mm"}, CurrentIndex: paperIndex(s.DefaultPaper)},
					Label{Text: "失败任务自动重印:"},
					CheckBox{AssignTo: &a.setAutoRetry, Text: "打印失败后自动重试一次", Checked: s.AutoRetry},
				},
			},
			GroupBox{
				Title:  "云端 MQTT",
				Layout: Grid{Columns: 2, Spacing: 8, Margins: Margins{Left: 12, Top: 10, Right: 12, Bottom: 10}},
				Children: []Widget{
					Label{Text: "Broker:"},
					LineEdit{AssignTo: &a.mqttBroker, Text: s.MQTT.Broker, CueBanner: "mqtt.example.com"},
					Label{Text: "端口:"},
					LineEdit{AssignTo: &a.mqttPort, Text: strconv.Itoa(s.MQTT.Port)},
					Label{Text: "用户名:"},
					LineEdit{AssignTo: &a.mqttUser, Text: s.MQTT.Username},
					Label{Text: "密码:"},
					LineEdit{AssignTo: &a.mqttPass, Text: s.MQTT.Password, PasswordMode: true},
					Label{Text: "主题:"},
					LineEdit{AssignTo: &a.mqttTopic, Text: s.MQTT.Topic, CueBanner: "print/jobs"},
				},
			},
			Composite{
				Layout:   HBox{},
				Children: []Widget{HSpacer{}, PushButton{Text: "保存设置", MinSize: Size{Height: btnHeight}, Font: btnFont, OnClicked: a.onSaveSettings}},
			},
			VSpacer{},
		},
	}
}

func paperIndex(w int) int {
	if w == 58 {
		return 0
	}
	return 1
}

func (a *App) onSaveSettings() {
	s := &a.cfg.Settings
	s.ServiceName = strings.TrimSpace(a.setName.Text())
	s.ListenPort = strings.TrimSpace(a.setPort.Text())
	s.APIPort = strings.TrimSpace(a.setAPIPort.Text())
	if a.setPaperCB.CurrentIndex() == 0 {
		s.DefaultPaper = 58
	} else {
		s.DefaultPaper = 80
	}
	s.AutoRetry = a.setAutoRetry.Checked() // 保留为新建打印机默认;每台重打由属性页控制
	s.MQTT.Broker = strings.TrimSpace(a.mqttBroker.Text())
	s.MQTT.Port = atoiOr(a.mqttPort.Text(), s.MQTT.Port)
	s.MQTT.Username = a.mqttUser.Text()
	s.MQTT.Password = a.mqttPass.Text()
	s.MQTT.Topic = strings.TrimSpace(a.mqttTopic.Text())

	a.save()
	a.flash("设置已保存")
}

func atoiOr(s string, def int) int {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return def
	}
	return n
}
