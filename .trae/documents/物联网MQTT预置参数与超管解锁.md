# 物联网 MQTT 预置参数 + 参数隐藏 + 超管解锁(含「订阅/上报」不显示修复)

## 1. 摘要

三项改动,全部落在 Windows 设置页(云端 MQTT)与配置初始化链路:

1. **预置物联网参数**:默认 Provider 改为 `iot`;MQTT 地址/端口/下行后缀/上行后缀在配置初始化时写死默认值(端点 `iot-060a5ivg.mqtt.iothub.aliyuncs.com`、端口 1883、下行 `pushMsg`、上行 `self_service_reply`);旧配置启动时一次性迁移。
2. **参数隐藏 + 超管解锁**:默认态接入方式只显示「物联网平台」、5 个参数行隐藏(地域/地址/端口/上下行后缀);新增「超管配置」按钮,输入 `88888888` 后当前会话内完整恢复(三接入方式可选、5 参数可编辑),重启回落。
3. **修复上轮 UI 改坏的「订阅/上报」预览不显示**:根因是上轮给预览 Label 加 `EllipsisMode: EllipsisEnd` 后,其 `MinSize().Width` 归 0 且 LayoutFlags 无 `GrowableHorz`,在 Grid 列宽分配中被压成 0 宽不可见。

## 2. 现状分析(已核实的机制)

### 2.1 配置初始化链路

* [Config.go Load()](file:///e:/congmingpay-print/internal/config/Config.go#L72-L86):文件不存在 → `Default()`(内存默认,不落盘);存在 → unmarshal 到 `Default()` 值上 + `normalize()` 补全。

* 默认值唯一出处 [Settings.go DefaultSettings()](file:///e:/congmingpay-print/internal/model/Settings.go#L87-L99):当前 `Provider: MQTTProviderGeneric`、`Iot: IoTMQTT{Port:1883, RegionId:"cn-shanghai"}`,**Endpoint/DownSuffix/UpSuffix 无默认值**。

* 落盘点:UI「保存设置」`a.save()`([App.go:331](file:///e:/congmingpay-print/internal/ui/App.go#L330-L333))与云端登记打印机。

* 连接拼接 [Iot.go](file:///e:/congmingpay-print/internal/mqtt/Iot.go):`ResolveIotEndpoint` **手填 Endpoint 优先**(预置端点 `iot-*.mqtt.iothub.*` 会被原样使用,地域字段空着也不影响);主题 `/{pk}/{dn}/user/{后缀}`;`Port≤0` 回退 1883。

### 2.2 「订阅/上报」不显示的根因(上轮 800×600 改造引入)

* [static.go:306-347](file:///C:/Users/a1115/go/pkg/mod/github.com/lxn/walk@v0.0.0-20210112085537-c389da54e794/static.go#L306-L347):Label 设 `EllipsisEnd` 后 `LayoutFlags()=ShrinkableHorz`、`MinSize().Width=0`;**没有 GrowableHorz**。

* [gridlayout.go:609-624](file:///C:/Users/a1115/go/pkg/mod/github.com/lxn/walk@v0.0.0-20210112085537-c389da54e794/gridlayout.go#L609-L624):`PerformLayout` 对 `GrowableHorz==0` 的 item,`w = minSizeEffective().Width`(被 MaxSize 钳制)→ **EllipsisEnd Label 宽度被压成 0**,文字完全不显示。

* 受影响:IoT 表单「订阅/上报」「接入点/User/CID」行(截图红框)。阿里云表单的预览 Label 有同样隐患(列宽尚足,但同样应修)。

### 2.3 MQTT 页现状

* [SettingsView.go](file:///e:/congmingpay-print/internal/ui/SettingsView.go):接入方式行 3 个 RadioButton(`mqttProvGeneric/Ali/Iot`)+ `showMqttForm` 按选中显隐三组 GroupBox;IoT 表单 10 行 Grid(每行 Label+控件,含 2 个 HBox 容器);底部 连接测试/保存设置。

* 对话框模式:项目统一 `(Dialog{...}).Run(a.mw)` 一步式(参考 [Properties.go](file:///e:/congmingpay-print/internal/ui/Properties.go));密码框用 `LineEdit{PasswordMode: true}`。

## 3. 具体改动

### 3.1 internal/model/Settings.go — 预置默认值

`DefaultSettings()` 的 MQTT 段改为:

```go
MQTT: MQTT{
    Port:     1883,
    Provider: MQTTProviderIot, // 默认物联网平台(预置端点+后缀)
    Aliyun:   AliyunMQTT{Port: 1883},
    Iot: IoTMQTT{
        Port:       1883,
        Endpoint:   "iot-060a5ivg.mqtt.iothub.aliyuncs.com",
        DownSuffix: "pushMsg",
        UpSuffix:   "self_service_reply",
    },
},
```

* 移除 `RegionId: "cn-shanghai"`(Endpoint 已预置且优先,地域行已隐藏,不留无用默认)。

* 同文件新增两个常量(供 config 迁移与 UI 超管按钮共用):

  ```go
  // Iot 预置参数(预置端点下地域/后缀固定;超管解锁后可改)。
  const (
      IotDefaultEndpoint = "iot-060a5ivg.mqtt.iothub.aliyuncs.com"
      IotDefaultDownSuffix = "pushMsg"
      IotDefaultUpSuffix = "self_service_reply"
  )
  ```

### 3.2 internal/config/Config.go — normalize() 补全(旧配置迁移)

`normalize()` 追加:

```go
// 云端通道唯一且默认为物联网平台:旧配置非 iot 时切为 iot,并补全预置参数。
// Endpoint/后缀仅在空时补预置值——超管解锁后用户改过的值不被覆盖。
if c.Settings.MQTT.EffectiveProvider() != model.MQTTProviderIot {
    c.Settings.MQTT.Provider = model.MQTTProviderIot
}
if c.Settings.MQTT.Iot.Endpoint == "" {
    c.Settings.MQTT.Iot.Endpoint = model.IotDefaultEndpoint
}
if c.Settings.MQTT.Iot.DownSuffix == "" {
    c.Settings.MQTT.Iot.DownSuffix = model.IotDefaultDownSuffix
}
if c.Settings.MQTT.Iot.UpSuffix == "" {
    c.Settings.MQTT.Iot.UpSuffix = model.IotDefaultUpSuffix
}
```

* 语义:新机器 Default() 已带预置值,无需迁移;旧机器启动即切 iot + 补参数,用户点「保存设置」后落盘(等价于"初始化写死");超管改过的值永不被重置。

### 3.3 internal/ui/SettingsView\.go — 隐藏行 + 超管按钮 + 预览修复

**(a) App 结构体新增字段**([App.go](file:///e:/congmingpay-print/internal/ui/App.go)):

```go
mqttProvRow    *walk.Composite // 接入方式行(整行显隐;锁定态隐藏)
mqttIotRegion *walk.Composite // IoT 表单 5 个隐藏行的容器(整行 Composite,整行显隐)
mqttIotEndpoint *walk.Composite
mqttIotPort     *walk.Composite
mqttIotDown     *walk.Composite
mqttIotUp       *walk.Composite
```

注:Grid 中"整行显隐"要求把每行的 Label 与控件一起包进一个 `Composite{Layout: HBox{MarginsZero: true, Spacing: 6}, AssignTo: &a.mqttIotXxx, Visible: false}`(行 = 该 Composite 一个 Grid 单元格,占列 2)。

**(b)** **`mqttPageWidget()`**:

* 接入方式行整体包进 `Composite{AssignTo: &a.mqttProvRow, Visible: false, Layout: HBox{...}}`(内含 Label"接入方式:" + 3 个 RadioButton + HSpacer)。锁定态整行隐藏。

* 底部按钮行改为:`[超管配置] [HSpacer] [连接测试] [保存设置]`。超管按钮 `OnClicked: a.onMqttSuperAdmin`。

**(c)** **`mqttFormIot()`** **隐藏 5 行**:地域/MQTT地址/端口/下行后缀/上行后缀 每行改为 `Composite{AssignTo: &a.mqttIotXxx, Visible: false, Layout: HBox{MarginsZero: true, Spacing: 6}}` 包裹原 Label+LineEdit,其余 4 行(3 输入行 + 2 预览行)不变。

**(d) 超管解锁(新函数,密码常量 88888888)**:

```go
const mqttSuperAdminCode = "88888888"

func (a *App) onMqttSuperAdmin() {
    var dlg *walk.Dialog
    var codeLE *walk.LineEdit
    ok := false
    _ = (Dialog{
        Title:  "超管配置",
        MinSize: Size{Width: 320, Height: 120},
        Layout:  VBox{Spacing: 10},
        Children: []Widget{
            Label{Text: "输入超管密码:"},
            LineEdit{AssignTo: &codeLE, PasswordMode: true, MaxSize: Size{Width: 200}},
            Composite{Layout: HBox{Spacing: 8}, Children: []Widget{
                HSpacer{},
                PushButton{Text: "确定", OnClicked: func() {
                    if strings.TrimSpace(codeLE.Text()) == mqttSuperAdminCode {
                        ok = true
                    }
                    dlg.Accept()
                }},
                PushButton{Text: "取消", OnClicked: func() { dlg.Cancel() }},
            }},
        },
    }).Run(a.mw)
    if !ok || strings.TrimSpace( /* codeLE 已随对话框销毁,靠闭包 ok 判定 */ ) == "" && !ok {
        return
    }
    if !ok { a.warn("超管配置", "密码错误。"); return }
    a.applyMqttUnlock(true)
}
```

注意项目坑:关闭后不能读控件,校验必须在 `Accept()` 前用闭包变量 `ok` 完成。

`applyMqttUnlock(unlocked bool)`(会话态,不写配置):

```go
a.mqttProvRow.SetVisible(unlocked)
a.mqttIotRegion.SetVisible(unlocked)
a.mqttIotEndpoint.SetVisible(unlocked)
a.mqttIotPort.SetVisible(unlocked)
a.mqttIotDown.SetVisible(unlocked)
a.mqttIotUp.SetVisible(unlocked)
if unlocked { a.showMqttForm(a.currentProvider()) } // 恢复三表单显隐逻辑
```

* `Run()` 返回时对话框已销毁,直接按 `ok` 判断即可。

* 重启后 `mqttPageWidget` 重建时默认 `Visible:false`,自动回落锁定态(无需持久化,符合"当前会话内")。

**(e) 「订阅/上报」预览修复(根治,统一处理)**:
所有动态预览 Label(`aliPreviewSub/Rep/CID`、`iotPreviewSub/Rep/Host/User/CID`、`mqttStatus`)在 `EllipsisMode: EllipsisEnd` 基础上追加 `MaxSize: Size{Width: 300}`:

* 根因修复:有 MaxSize → `minSizeEffective` 给到 300 → 该 item 在 Grid `PerformLayout` 中 `GrowableHorz==0` 分支 `w = min(300, 列宽)` ≥ 300,文字恢复可见;且 300 ≤ 列宽(\~338),不会把页面最小宽顶过 686 内容宽,上轮"切页不撑宽"的成果保留。

* 两个预览 HBox 容器(「订阅/上报」行、「接入点/User/CID」行)及阿里云「实际上报/ClientID」行,HBox 末尾各加 `HSpacer{}`:让容器 LayoutFlags 带 `GrowableHorz|GreedyHorz`,Grid 该列 `MinSize>0`(spacer 0 宽但贪婪参与分配),列宽分配稳定,行尾弹性留白。

### 3.4 不改的部分

* 常规页 `docURL` 的 `EllipsisEnd`:所在 Grid 列由其他行(复选框\~280)撑宽,URL 文本远小于列宽,无塌缩,保持原样。

* `validateMQTTForSave`/`currentIot`/`onIotPreview`/`showMqttForm` 逻辑不变:锁定态隐藏控件的 Text 保留初始值,点「保存设置」只会把预置参数原样写回,不会破坏。

## 4. 假设与决策

| 决策            | 取值                                                      | 依据                                    |
| ------------- | ------------------------------------------------------- | ------------------------------------- |
| 旧配置迁移         | 启动时 normalize 一次性切 iot + 空值补预置                          | 用户确认:目前无其他通道设备,三种链接互斥,默认物联网           |
| 预置端点是否每次强制覆盖  | 否,仅空时补(Endpoint 已预置的旧配置保留)                              | 超管改过后不被重置                             |
| 解锁时效          | 当前会话(重启回落)                                              | 用户已确认                                 |
| 解锁范围          | 完整恢复(三接入方式 + 5 参数)                                      | 用户已确认                                 |
| 密码            | 常量 `88888888` 硬编码                                       | 用户给定,无安全存储需求                          |
| "写死在数据库中"     | 落点 = DefaultSettings + normalize 迁移 + 保存时落盘 config.json | 项目无独立数据库,config.json 即配置存储;效果等价且兼容旧配置 |
| 预览 Label 宽度上限 | MaxSize 300                                             | 列宽 \~338 内,防撑页(686 预算)同时保证可见          |

## 5. 验证步骤

1. `go build ./...` 编译通过。
2. 新建目录(无 config.json)启动:进「云端 MQTT」页——应只有 启用开关 + 物联网表单(ProductKey/DeviceName/DeviceSecret 三行 + 订阅/上报、接入点/User/CID 两预览行,**预览行有正常宽度可见**)+ 超管配置按钮;无接入方式行、无地域/地址/端口/后缀行;窗口保持 800×600,四页来回切换宽度不变。
3. 填 PK/DN/Secret 后「保存设置」→ 退出重启:三输入值保留、Provider=iot、端点/后缀为预置值;连接状态可正常连接(端点预置生效)。
4. 超管配置:输错 → 提示"密码错误";输 88888888 → 展开接入方式行(三选项可切)与 5 参数行可编辑;切 Provider 表单跟随;重启后回落锁定态。
5. 旧配置回归:把现有 config.json(Provider=generic/aliyun)放旁边启动 → 自动切 iot、参数补预置,保存后落盘;超管改过端点后再重启 → 端点保持用户改的值。

