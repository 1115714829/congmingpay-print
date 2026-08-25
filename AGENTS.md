# AGENTS.md

This file provides guidance to Codex (Codex.ai/code) when working with code in this repository.

## 项目定位

面向**餐厅厨房**的**热敏小票打印服务器**。完全独立项目,与本机其他项目(`F:\Nitebot-OTA`、`F:\dianxinxiansu` 等)无关,不复用其代码或假设。

- 目标设备:**58mm / 80mm 热敏小票打印机**,主用 **ESC/POS** 指令。
- 最终形态:**云端下发指令 → 本地服务执行 → USB/网口打印**。
- 长期目标:**多品牌**、**USB + 网口**多通道。
- 目标平台:**Windows 与 Android,两端独立开发**(不做跨平台共享核心)。Android 以后单独进行。

## 当前阶段:多打印机管理控制台

按 Codex Design 设计稿(`50-80.zip` → `_design/58-80/project/打印服务器控制台.dc.html`)用**原生 Walk** 重构为「票据打印服务器 — 管理控制台」:

- **左导航**(打印机 / 打印队列 / 系统设置)+ 内容区切换 + **底部状态栏**(`共 N 台 · N 在线 · N 任务进行中` + 临时消息)。
- **打印机视图**:工具栏(新增/刷新/测试打印/打印样票/**JSON测试**〔填 JSON 单对象或数组,本地直接走 `api.Process`,等价 MQTT 下发〕/属性/删除)+ 筛选(全部/58/80/在线)+ 搜索 + 多列 `TableView`(名称/来源〔local 本地添加·cloud 云端下发〕/品牌/规格/连接/地址/彩色状态/**上次打印时间**〔成功出纸绝对时间〕)。
- **打印队列视图**:任务表(预览/重新打印/取消/清除已完成),状态彩色;**任务落盘 `jobs.db`(SQLite,modernc 纯 Go 无 CGO)**,重启恢复未完成并续打;JSON 任务另存 `source_json` 供「预览」出图(`internal/preview`)。
- **系统设置**:服务名(**用作窗口标题与托盘提示**,保存即时生效、空回退默认) + **系统通知开关**(`NotifyDisabled` 反向字段,默认开,保存即时生效) + **云盒兼容模式**(`YunheCompatDisabled` 反向字段,默认开=C1–C4,保存即时生效;见 [`旧版兼容.md`](旧版兼容.md)) + **打印历史保留天数**(`JobHistoryDays`,默认7,仅清 done/failed) + **云端 MQTT**(Tab 三选一:`generic` 自建 · `aliyun` 云消息队列 · `iot` 物联网一机一密;`Resolve`统一产出;**启用时校验当前 Tab**) + **本地接口文档服务**(启用/端口/局域网地址,只读文档) + 保存。
- **新增打印机三步向导** + **属性对话框**。
- **多打印机 + 并发打印**:每任务独立 goroutine;**离线转「等待重试」队列、联通即打**(唯一自动重试,仅连接类、不限次数);刷新时并发查所有打印机状态。

设计稿是自定义样式 web mock,原生 Walk 还原**结构/交互/彩色状态**,视觉是标准 Win32(非 mock 的浅蓝 Fluent/圆角/开关)。

> 云端通道 = **MQTT(唯一,已实现,`internal/mqtt`)**:Provider 三选一——**自建**(用户名密码,订阅短商户号,上报手填主题)、**云消息队列 MQTT 版**(AK/SK;`GroupId@@@自定义ID`;主题 `{父主题}/{自定义ID}/{上下行后缀}`)、**物联网平台**(一机一密 ProductKey/DeviceName/DeviceSecret;主题 `/{pk}/{dn}/user/{上下行后缀}`;设备列表预留)。payload=`PrintRequest` 单/数组收打印;上行 **report** / **state**(定时+查询+LWT);每条带 `merchant`(自建=短商户号,云消息队列=自定义ID,物联网=DeviceName)。**数据 HTTP 通道已移除**;另有 `internal/docserver` 只读接口文档页。

## 技术栈(已定)

- **语言:Go。发布构建锁 Go 1.20**(最后一个支持 Windows 7 的 Go 版本)。产物是**单个静态 exe,目标机零运行时依赖**。
  - **决定能否跑 Win7 的是实际编译的工具链版本,不是 `go.mod` 的 `go 1.20` 指令行。** 用高版本 go 编译(即便 go.mod 写 1.20)产物在 Win7 起不来。
  - **不降级开发机默认 Go**:官方并行安装 `go install golang.org/dl/go1.20.14@latest` + `go1.20.14 download`,发布用 `go1.20.14 build`(或 `GOTOOLCHAIN=go1.20.14`)。日常 `go vet`/gopls 用默认高版本 Go 无妨。
- **UI:Walk(`github.com/lxn/walk`)**,原生 Win32 窗口 + 托盘,单 exe,Win7 可用。
- **构建架构:GOARCH=386**(32 位二进制在 Win7/10/11 的 32&64 位上通吃);后续需要再加 amd64。
- **运行权限:普通用户(`asInvoker`)**。spooler 打印与 TCP 都不需要管理员。
- **强制支持 Windows 7 → 最新版本。**

## 代码规范(项目规则)

- **命名:英文大小驼峰。** 标识符(类型/函数/方法/变量/常量)用驼峰:**导出 `PascalCase`,非导出 `camelCase`**(与 Go 惯例一致)。文件名也用**大驼峰**(如 `MainWindow.go`、`Printer.go`)。
- **模块化:按职责拆包、拆文件,不把代码堆进单文件。** 工具/通用部分与私有实现分离;`tree` 要能一眼看清各模块。
- **平台专属文件用 `//go:build windows` 标签**,不用 `_windows.go` 后缀,保持文件名为驼峰。
- **Go 硬性例外**:包目录名必须小写(如 `config`/`transport`/`escpos`/`ui`/`util`),这是 Go 规定,不与驼峰规则冲突。

## 文档规范(硬性,用户明确要求)

适用于三份对外标准文档:`接口文档.md`(权威接口规范)、`internal/docserver/apidoc.html`(同步 HTML 呈现)、`功能说明.md`(产品/操作手册):

- **只陈述现行为。** 禁止口语与第二人称指令(「你/请去看/打完去」),禁止编辑性口头补充(「有据可查」类)。
- **禁止历史相对措辞**:「早期版本」「旧版本」「升级时」「不再」「已移除」「曾」「已修」等一律不得出现——行为变更后直接改写为新行为的陈述句,变迁过程只允许进《修订记录》表。
- 术语统一(打印回执(report)/状态快照(state)/错误码(code)/本服务/打印机/短商户号/品牌「其他」);正文全角标点,字段名/取值/JSON 用代码体;字段表统一五列(字段/类型/必填/默认/说明)。
- 例外:`测试JSON.txt` 刻意保留口语操作指南风格(仅校对事实);AGENTS.md 本身为内部开发备忘,不受上述文风限制。
- **不得替用户臆定协议参数**:云端相关的固定参数(如上报主题)无默认值、不自动派生,由用户手动填写;拿不准的值先问用户,不要以示例值写死。

## 架构(分层,为多品牌/多通道/云端铺路)

- **传输层 `internal/transport`**:接口 `Printer{ Open/Write/Close }` + `Print(p, data)` 便捷封装。
  - `Spooler.go`(`//go:build windows`,**USB/驱动**):经 `github.com/alexbrainman/printer` 走 winspool —— `ReadNames` 列已装打印机、`Open`+`StartRawDocument`(自动选 RAW/XPS_PASS)+`Write`+`EndDocument` 发**原始字节**。`ListRealPrinters()`(过滤虚拟打印机)供 UI 枚举。
  - `Tcp.go`(**网口**):`net.DialTimeout("tcp", ip+":9100")` 直发。`RawPrintPort=9100`。**拨号失败分两类**:`isConnRefused`(RST=主机在但 9100 关/未就绪/被占)→ `ConnError{Refused:true}`;其余(超时/不可达)→ 普通 `ConnError`。〔**踩过的坑**:Windows 上 `errors.Is(err, syscall.ECONNREFUSED)` 命不中——`syscall.ECONNREFUSED` 在 Windows 是另一 sentinel,底层实际是 `syscall.Errno==10061`(WSAECONNREFUSED);故 `isConnRefused` 既比 `ECONNREFUSED`(Linux 111)也比数值 10061〕。`transport.IsRefused(err)` 供 printsvc 区分**占用短退避**(RST)与**离线退避**(超时)两档等待重试。
  - `Status.go` / `SpoolerStatus.go` / `Ping.go`(**在线/状态检测**):**高频巡检(1s)网口走 `Ping.go` 的 `QueryICMPPing`——Windows `IcmpSendEcho`(iphlpapi.dll,普通权限,不碰 9100),ping 通=在线**;失败原因精确译码:`IcmpSendEcho` 返回 0 时取 `.Call` 第三返回值(syscall.Errno=GetLastError)得 IP_STATUS 码,`ipStatusText` 译常见码(11002 网络不可达/11003 主机不可达〔ARP·路由〕/11005/11010 请求超时〔拥塞丢包〕/11013/11018,中文+数字;回复 Status 非 0 同表译码,其余显数字);`PrinterStatus.RTTms`(仅 ping 成功有效)供监测丢包统计求均值,不上报云端。USB 走 winspool `GetPrinter`(经 `golang.org/x/sys/windows` 直调)。**4000 端口的缺纸/开盖详细状态检测(网口 `ESC v` 走端口 4000)已按用户要求彻底移除**(原 `QueryTCPStatus`/`StatusPort`/`parseGprinterStatus` 及 `PaperNearEnd`/`Raw` 字段,连同死代码 `QueryTCPOnline` 一并删除)——**全部通道统一只有在线/离线两态(用户拍板「USB 和网线IP一样」)**:网口=ICMP;USB=转述打印后台脱机标志(`STATUS_OFFLINE`/`NOT_AVAILABLE`/`WORK_OFFLINE`,detail=「打印后台正常/打印后台标记脱机」),缺纸/开盖/错误子状态位与 `summarizeStatus`、`PrinterStatus.PaperOut/CoverOpen/Error`、UI「缺纸/错误」标签全部删除,臆测口吻的驱动能力提示一并删除。**USB「关机但线仍插着」时系统常不标脱机**——判别看系统「设备和打印机」是否显示脱机,spooler 说就绪程序只能得到就绪;突破此限制需 SetupAPI 物理在场检测(见待确定)。
- **指令层 `internal/escpos`**:`Builder`(链式)构建 ESC/POS——`1B 40` 初始化、`1B 61 n` 对齐、`1B 45 n` 加粗、`1D 21 n` 倍宽高、`1D 56 01` 半切、`0A` 换行;**中文经 `util.ToGB18030` 编码**。`BuildTestReceipt(now, widthMM)` 生成测试小票(按纸宽自适应)。**纸宽自适应 `Layout.go`**:`CharsPerLine`—58mm=384点=32字符、80mm=576点=48字符(据佳博 demo/手册,标准行模式无需设宽指令)。**重印抬头 `Reprint.go`**:`ReprintBanner(widthMM)` 生成醒目「重打小票/打印服务器重印」,由 `printsvc` 在 `entry.reprintNext`(手动「重新打印」或消息 `reprint:1`)时前置到小票。蜂鸣见 `Buzzer.go`:`BuildBuzzer`=`ESC B n t`(手册命令62);报警灯扩展(`ESC C`,命令63)未接入、代码已清理,需要时按手册实现。蜂鸣属厂商扩展指令,多品牌接入时需适配。二维码 `QRCode.go`=`GS ( K` Model 2(模块大小参数化);`SetSize`(GS ! 放大)、`Barcode.go`(CODE128/CODE39 = GS k + GS h/w/H;**Code128 码集参数化 `{A}`/`{B}`/`{C}` 前缀**)、`CashDrawer`(ESC p m t1 t2,开钱箱)、`FeedDots`(ESC J n 点数走纸)、`Raster.go`(图像→单色位图 GS v 0,含 Floyd-Steinberg 抖动)、`Raw`(嵌原始字节)、`DisplayWidth`(中文=2 列宽)、`WidthDots`(58→384/80→576)。
- **JSON 排版渲染 `internal/layout`**:`Render(contents, widthMM)` 把云端 **MQTT type=0 的 JSON 小票排版数组**渲染为 ESC/POS。**全部严格模式(用户拍板)**:字段缺失=文档声明默认(合法);字段存在但非法 → **整单拒绝**,error 带 `contents[i](种类)` 定位(深层函数只报原因,`Render` 主循环统一包前缀),ack ok:false + 落日志,不产出部分字节。合法值:size=两位 0-7(text/表格/both_sides)、align=left/center/right、表格列宽 1-100 整数**总和恰 100**、行单元格数=列数、单元格仅标量、both_sides 恰 2 段、**qrcode size=两位 01-09(缺省 04,控制模块大小)**、**条码 size=两位 AB(A 模块宽 1-6 夹取≥2 / B 高度档 0-9,高度=(B+1)×24 点)**·hri 0-3、**bc128/bc128a 仅大写字母+数字且 ≤14 位、bc128c 仅数字且 ≤28 位**、**qrcode 内容 ≤512 字节(单/双码每段)**、png 须 http(s)/dataURI 且解码后 ≤60K、bmp 须 24/32 位未压缩 BMP 且 ≤6K(自研 `DecodeBMP`,无新依赖;Android 用 BitmapFactory+BM 魔数校验)、plugin cont 0/1 开钱箱(`ESC p`,t1/t2=25)、未知元素 type 拒(**compat=false 时 `cut` 也按未知 type 整单拒**——切纸只由顶层 `cut` 字段控制;**compat=true 时 cut 元素接受:cont "0"=全切、"1"=半切,均触发切纸**,false/"off" 保留不切)、有 tbody 无 thead 拒。支持元素:`title`(居中二倍字**不加粗**)/`text`(**cont 数组=多行**,每行同样式)/`div_line`·`div_star`/表格(**line_space=ESC J 点数**,8点=1mm)/`both_sides`/`qrcode`(单码 native、双码位图合成)/`bc128`·`bc128a`·`bc128c`(码集 A/A/C)·`code39`/`png`/`bmp`/`plugin`/`cut`(仅 compat)。`Sample.go` 内嵌样例(**png/bmp 为内嵌 data URI**——严格模式下外网 URL 断网会整单拒印),UI「打印样票」验证;`Render_test.go` 锁定全部拒绝点 + 样例(含 测试JSON.txt fixture)必须通过。依赖 `github.com/skip2/go-qrcode`(仅双码位图)。
- **消息处理核心 `internal/api`**(无 HTTP,只留复用逻辑):`Process(cfg,svc,*PrintRequest) (*ProcessResult{JobNo,Printer 快照}, changed, err)`(错误带 `internal/errcode` 全局错误码,「渲染失败: %w」保码) + `ParseRequests(raw) ([]PrintRequest, wasArray, err)` + `resolveTarget` + 请求模型(`PrintRequest`/`PrinterRef`)。供 **MQTT** 与 UI「JSON测试」复用。**HTTP 数据传输已移除**(原 `Server.go`/`Swagger.go`/`Printers.go`/`/api/*`/`/swagger` 删除,build.ps1 去 swag 步骤)——现只有 `internal/docserver` 的**只读文档** HTTP 页,不含任何数据/打印端点。
  - **打印消息(`PrintRequest`)**:目标二选一 `printer`/`gateway`;type〔**0** 或云盒兼容下 **5**=JSON 排版 / **1**=ESC base64〕;pWidth、pCopy、contents;**云盒兼容模式**(`Settings.YunheCompat`,默认开)控制 C1–C4:开=`type:5`、五参数可省、contents `cut`、MQTT 仅 `done`/`failed`;关=严格协议+完整 `accepted`/`waiting`。见 [`旧版兼容.md`](旧版兼容.md)。**payload** 可为单对象或数组,逐条 `Process`。
  - **随打印自动登记打印机(建列表)**:`resolveTarget` 目标**带 IP** 且**未注册** → `config.UpsertPrinter` 自动新增入库;**登记必须能确定纸宽:`printer.width`→`pWidth` 回退链,两者皆缺/非法即拒(不再兜底 80)**。`Process` 返回 `registered||changed`,调用方(MQTT/JSON测试)据此 `save`+刷新列表(state 由定时拍带出,不即时上报)。已注册机也被打印消息参数+身份覆盖并持久化。USB 与"仅名/ID"目标不自动登记。
  - **打印分支流程**:①带目标 → 未注册先自动登记 → `svc.Submit` 提交,`dispatch` 连接打印:在线即打、离线转「等待重试」队列(在线后自动打)。②无目标 → `Process` 报「未指定打印目标…」,调用方 `logger.Errorf` 落日志。
- **云端通道 `internal/mqtt`(唯一)**:`Client`(paho.mqtt.golang,MQTT 3.1.1)持 cfg/svc/cfgPath/onChange/onStatus(**id 去重已移除,收到即打**;`onPrint` 逐条 `Process` + 详细日志)。`Start/Reload(Settings.MQTT)/Close`;明文 + 用户名密码;**订阅 `<短商户号>`** 收打印 → `ParseRequests`→`Process`→回执;**发布到配置的「上报主题」(`ReportTopic`,与 broker/短商户号同级必要——启用而缺任一则只停不连;LWT 也发此主题;设置页启用时六项全必填拦截保存)**,两类上行且**每条都带 `merchant` 字段(短商户号,服务端单主题一对多靠它分辨来源)**:**打印回执 `report`**(`Report.go` `reportMsg{type:report,merchant,event,id〔恒回显〕,jobNo omitempty,code,message,printer*,params*,ts}`,event=accepted/waiting/done/failed 统一结构:accepted 由 `onPrint` 发〔code 0,带 `printer`=`ProcessResult.Printer` 身份 + `params`=本单生效参数(buzzer/cut/reprint/headLines/tailLines/pWidth 实际渲染宽/pCopy/contentType)〕;受理失败=failed〔code=`errcode.CodeOf(err)`,1xxx-4xxx;整条解析失败 id=0、code=1001〕;done/failed/waiting 由 printsvc `JobEvent` 经 `main` 装配 `mc.PublishJobEvent` 驱动〔done=code 0「打印成功」、执行失败=5001/5002、20s 卡单 waiting=5101〕)** + **状态快照 `state`**(`Report.go` `PublishState`,`stateMsg{type:state,merchant,online,printers:[printerId/printer/brand/width/conn/ip·port 或 usbName/online/detail/buzzer/cut/headLines/tailLines/source/lastPrint],ts}`,**全量幂等**=`cfg.PrinterSnapshots()` ⊕ `svc.OnlineSnapshot()` 合成;**定时上报(用户拍板,事件触发全部移除)**:OnConnect 上线首拍立即一条 + `stateLoop` 每 `stateInterval`(包级 var=1 分钟,测试临时调小)一条〔goroutine 随连接生命周期,`stopTick` 停;断线/未启用拍静默跳过不刷日志〕+ 下行**查询消息 `{"query":"printers"}`** 按需拉取一条(`onPrint` 先 `parseQuery` 分流:顶层 query 非空字符串=查询;未知 query 值 → report failed code=1002 UnsupportedQuery);状态/配置变化不即时上报、最坏滞后 1 分钟(出单不受影响——离线排队联通即打);**离线为简版 `{type:state,merchant,online:false}`**——LWT 遗嘱 + **Close/Reload 收尾时旧连接主动发**(`sendOffline` 同步短超时;干净 DISCONNECT 会丢弃 LWT,停用/换配置不主动发会让云端永久滞留 online:true);MQTT 未启用时静默跳过)。**`publish` 代际校验**:传入 cli ≠ 当前 c.cli(Reload 已换连接/Close 已关)即静默跳过——封死「Close 发完离线后在途 PublishState 又补发 online:true」的交错窗口;Close 置 enabled=false 静默收尾。**`publish` best-effort 但有据可查**:不排队不补发;未连接/主题空时若启用则 `logger.Errorf`,发布后异步查 token、失败 `logger.Errorf`(网络异常正常发送、日志输出失败原因——用户需求)。〔**踩过的坑**:paho v1.4.3 在 ConnectRetry 下「连接中」`IsConnected()` 也为 true,该窗口 QoS1 Publish 只进内部存储、token 永不完成,CleanSession 连上后 `persist.Reset()` 静默丢弃 + `tok.Wait()` goroutine 永久泄漏——故 publish 必须判 **`IsConnectionOpen()`**(仅真连上才 true)转「跳过+日志」,残余瞬断窗口用 **`WaitTimeout(30s)`** 兜底〕。**自订阅回环守卫**:上报主题 == 订阅主题(短商户号)会让 ack 被自己收到→解析失败→再发失败 ack 无限风暴——设置页拦截保存 + `reconnect` 拒连双层防护。`SetAutoReconnect`+`SetConnectRetry` 自动重连(重订阅在 `OnConnect` 内,重连自动恢复;订阅+上线 publish 放 goroutine 里做,不阻塞 paho 回调线程);`TestConnect` 供设置页「连接测试」;`SanitizeMerchant` 净化短商户号(仅 `[A-Za-z0-9_]`;上报主题不走净化,允许 `/`、禁通配符 `+`/`#`,设置页保存时校验)。**上报主题无默认值、无迁移**:服务端固定主题由用户手填(与 broker/短商户号同级必要,空即拒连;勿再引入任何自动派生/默认)。**ClientID=短商户号**〔用户拍板:同商户号多客户端由云端从根源杜绝,程序不做防重复处理、**勿再加随机后缀**;副作用:连接测试与主连接同 ClientID,测试时 broker 会替换主连接、靠自动重连恢复;背景知识:同 ClientID 顶号表现为「用一会就不收消息」〕。**`SetOnStatus` 状态回调**:`setConnected`/`setErr` 触发 `fireStatus`,UI marshal 回界面线程刷新 MQTT 状态标签 + `mqttNotifyEdge` 系统通知(解决「保存后卡在连接中…」——连接是异步的,靠回调而非保存时那一次即时读);**`Active()`**(cli≠nil)供 UI 判「未启用/已停用」中性态——Close/reconnect 置 cli=nil 不触发 onStatus。`main` 建 `mqtt.New(cfg,svc,cfgPath).Start()`,`defer Close()`;UI 在 `Run` 设 `onChange`+`onStatus`、设置保存时 `Reload`。**库版本**:`paho.mqtt.golang v1.4.3`(依赖仅 gorilla/websocket + x/sync,**不顶高 x/text v0.14/go1.20/Win7**;MQTT5 的 `paho.golang` 需 Go1.23+x/text0.21 **不可用**)。
- **数据模型 `internal/model`**:`Printer`(名称/**品牌**〔佳博/飞蛾/其他,佳博飞蛾同 ESC/POS 协议〕/规格/连接/IP·端口/USB驱动名/**来源** `Source`〔`local` 本地添加·`cloud` 云端下发;旧配置空=local〕/**上次打印** `LastPrint`〔成功出纸绝对时间 `2006-01-02 15:04:05`,空=未打过〕/**蜂鸣开关** `BuzzerEnabled`/**首尾空行** `HeadLines`(绝对)·`TailLines`(尾部**偏移**,实际=`escpos.TailFeed(width,offset)`=基数+偏移,**基数按纸宽:58=5、80=3**,偏移可负、下限0)〔走纸 LF,属性页设,标签动态显示基数〕/**切刀** `CutDisabled`(反向,`Cuts()`))、`Job`(任务+状态,状态含 `JobWaiting`「等待重试」,`Active()` 纳入)、`Settings`(服务名/**系统通知** `NotifyDisabled`〔反向,零值=启用〕/**云盒兼容** `YunheCompatDisabled`〔反向,零值=兼容开 C1–C4,`YunheCompat()`〕/**打印历史保留天数** `JobHistoryDays`〔默认7,范围1–365〕/MQTT/DocServer)。品牌当前仅元数据+未来适配预留,不分支打印逻辑。**每台重试字段(NoRetry/RetryMax)已移除**——重打逻辑只有「离线等待联通即打」+「手动/消息 `reprint` 抬头」,不再有每台有限重试。
- **打印服务 `internal/printsvc`**:`Service.Submit` 每任务开 goroutine → **并发打印**;`Retry`/`Cancel`/`ClearDone`;**任务持久化 `Store.go`/`Persist.go`**:exe 同目录 `jobs.db`(`modernc.org/sqlite`,无 CGO);Submit/状态变更 UPSERT、Cancel/ClearDone DELETE;`RestoreAndResume` 启动灌入(`JobPrinting`→`JobQueued` 再 dispatch);`PruneHistory`/`SetHistoryDays` 仅删超期 done/failed;`OpenStoreOrRecover` 坏库备份后空队列。`Submit(p,doc,data,*Options)` 的 `Options`(cut/buzzer/head/tail/**reprint** 指针/**`CloudID *uint32`**〔云端消息 id 随任务走,`api.Process` 填 `&req.ID`,nil=本地任务〕)**按需覆盖每台默认**(nil=默认),供 MQTT/JSON测试传参(`reprint` 是每单标记,不写回打印机);**任务事件 `Events.go`**:`SetOnJobEvent(func(JobEvent))`,`JobEvent{Event(done/failed/waiting),Code(errcode),JobNo,CloudID,Printer 值拷贝,Err}`——done/failed 由 `setStatus`(带 code 参数:done=0、网口写失败=5001、USB 提交失败=5002)锁内快照、锁外触发(四个终态出口全经 setStatus 收口各恰一次);waiting 由 `waitAndContinue` 的 warnNow(持续被拒超 `longWaitWarn`〔包级 var=20s,测试临时调小〕)触发、每单至多一次(code=5101,非终态),`main` 装配单回调槽并联两个消费者:`app.NotifyJobEvent`(系统通知,failed/waiting,本地任务也通知)+ `mc.PublishJobEvent`(CloudID==nil 的本地任务过滤不上报);**`EventDone` 时 UI 写打印机 `LastPrint` 并落盘 config**;回调契约:快速返回不阻塞;**在线注册表**:`SetPrinterOnline(id,online,detail)` / `OnlineSnapshot()`(`onlineMu` RWMutex)——StatusMonitor 每秒巡检写入保持新鲜,`state` 快照合成时读;`Status(p)` 是**唯一公共在线/离线检测入口**(网口 ICMP 不碰 9100 / USB winspool),监测、dispatch 判在线、新机监测三处共用(见下)。`dispatch` **统一组装收尾**:蜂鸣 → 重印抬头 → `FeedBytes(HeadLines)` → 内容 → `Finish(Width,TailLines,Cuts())`(尾部走纸 + 按切刀设置切纸)。**内容构建(`BuildTestReceipt`、`layout`)不再自带走纸/切纸**——避免切纸前走纸不足把底部内容切断,且每台「尾部空行/切刀」设置对测试小票与云端样票一致生效。`layout` 不识别 `cut` 元素(按未知 type 整单拒,切纸只由顶层 `cut` 字段控制)。`notify` 回调由 UI marshal 回界面线程。
  - **dispatch 状态机(判在线分支 + 真打裁决 + 两档退避)**:① **首次派发**先 `Status(&e.printer)` 判在线(仅网口 ICMP,不碰 9100;`entry.gated` 只门控一次)——离线则直接进 `JobWaiting` 省 5s 空拨号;**重试一律直接真打**(真打为最终裁决),避免 ICMP 被过滤但 9100 可打的机被假离线永久排队。USB 不做 ICMP 门控(交 spooler 排队)。② **真打**(`transport.Print` 一次性 Open→Write→Close)按错误分类:`err==nil`→`JobDone`;`transport.IsRefused`(RST=被占/端口未就绪)→ `JobWaiting`「**占用短退避**」(1s→2s→3s);`transport.IsConnError` 非 refused(超时/不可达)→ `JobWaiting`「**离线退避**」(5s→30s);其余(已连上但 Write 失败)→ `JobFailed`(避免重复出纸)。**RST 不再快速失败**(旧 `printerBusy` 快速失败已删)——被占/端口未就绪都排队等待,联通/释放即打。
    - **每台打印串行锁**(`Service.printLocks` = `printerID→*sync.Mutex`):**仅在真打那步持锁**(退避 sleep 不持锁),本进程对同一台同时只开一个 9100 连接——`pCopy` 多份/并发任务**串行**打印、彼此不自撞 RST;不同打印机仍并行。有此锁后自撞不可能,故 RST 必是**第三方占用或端口未就绪**,统一排队等待(`printerBusy`/`busy-guard` 已随快速失败一并删除)。
    - **坏端口/错IP 可见告警(不静默丢单)**:占用流连续被拒超 `longWaitWarn`(20s)→ 把 `job.Err` 置「长期等待·疑似端口未就绪/故障」并**节流** `logger.Errorf` 一次;任务**保持 `JobWaiting` 永不自动失败**。UI 打印队列「详情」列展示该告警(红色,`Tables.go` `JobModel` col 5——份数列已删,列号:任务号0/文档1/打印机2/状态3/时间4/详情5)。
  - **上线/释放即打**:等待中的退避 `select` 同时监听 `entry.wake`;UI 每台持续 ping 检测到某机**上线**(离线→就绪)即调 `Service.NudgeOnline(printerID)` 唤醒它所有 `JobWaiting` 任务、重置退避**立即重打**(~1s)。离线走此路径秒打;**占用释放**(ICMP 全程在线、无边沿,不触发 nudge)靠**占用短退避**(~1-3s)再打抓取——这是「空闲零接触 9100」下的取舍(真「秒级」需探 9100,已被用户否)。即"打印在在线/离线判定之后:不通/被占进队列,通了/释放了就打"。
  - **「重打」抬头两种触发**:`entry.reprintNext` 决定 `dispatch` 是否前置 `ReprintBanner`——**① 用户点「重新打印」(`Retry`)时置位;② 打印消息 `reprint:1`(经 `Options.Reprint`)首次派发即置位**。连接等待重试不改此位(联通即打时若原为 reprint:1 仍带抬头,否则不带)。
- **配置 `internal/config`**:本地 JSON(与 exe 同目录 `config.json`)—— `Settings` + `Printers[]`(持久化打印机注册表)。**打印机 ID 用 `NewPrinterID()`=时间戳+原子递增序号**〔**踩过的坑**:原来用裸 `time.Now().UnixNano()`,Windows 时钟精度粗(~100ns),数组一次登记多台时并发两次调用撞出**相同 ID**,状态表按 `p.ID` 存取导致两台状态互相绑定(关一台另一台跟着离线);FindPrinter/RemovePrinter 也会误取。**AddWizard/UpsertPrinter 一律走 `NewPrinterID`,勿再用裸 UnixNano**〕。**config.json 损坏处理**:`main` 把坏文件备份为 `config.json.bad-<时间戳>` 并落日志,再用默认配置启动(打印机注册表可从备份手工恢复)——这是损坏健壮性,非兼容逻辑;**配置无任何迁移/自愈代码**(见工作规则「开发期无兼容」)。
- **单实例 `internal/instance`**:`Acquire()` 命名互斥体 `Global\congmingpay-print-server`(**Global=跨用户会话唯一**——打印机/端口/MQTT ClientID 是机器级资源;普通用户可建 Global 互斥体,SeCreateGlobalPrivilege 只限 section/符号链接)。`main` 在日志横幅后、**config 加载前**守卫(二次启动不触碰配置/MQTT/端口——避免同 ClientID 顶号与损坏备份竞争):已存在 → 落日志 + `walk.MsgBox(nil,…)` 友好提示「程序已在运行」后退出(**标题用 `config.PeekServiceName` 只读窥探服务名**——不备份不改写零副作用,与运行实例的窗口标题/托盘提示一致);互斥体创建失败按守卫失效继续启动(不因守卫挡打印服务)。**x/sys 的 `CreateMutex` 在 ERROR_ALREADY_EXISTS 时返回「有效句柄+该错误」**(zsyscall 特判),重复路径须 CloseHandle;**ERROR_ACCESS_DENIED 也判「已在运行」**(49 代理审查抓出的高危:nil 安全属性互斥体默认 DACL 只授创建者+SYSTEM,**换用户会话二启拿到的是 ACCESS_DENIED 而非 ALREADY_EXISTS**,不映射会被当守卫失效放行双实例);只用存在性判重不取所有权(`initialOwner=false`,无 abandoned 态)。`Lock.Release` nil 安全,进程退出 OS 自动回收。
- **日志 `internal/logger`**:全局文件日志(与 exe 同目录 `congmingpay.log`)。**业务代码一律调用 `logger.Info/Infof/Warn/Warnf/Error/Errorf`,不直接用标准库 `log`**;`main` 启动时 `logger.Init(path)`。GUI 无控制台,靠此排查。
- **工具 `internal/util`**:`ToGB18030`(x/text 编码)。
- **资源 `internal/assets`**:`//go:embed app.png` 图标,保证单 exe。
- **接口文档服务 `internal/docserver`(仅局域网、只读)**:标准库 `net/http`,`//go:embed apidoc.html`。**只暴露 `GET /` 返回内嵌接口文档页;其它路径 404、其它方法 405;无任何打印/数据端点**(数据只走 MQTT,不越权)。`Server.Start/Reload/Stop(model.DocServer)`;`LANIP()` 取私网 IPv4 供 UI 展示访问地址。配置 `Settings.DocServer{Enabled,Port}`(默认启用、8080)。`main` 建 `docserver.New().Start()`、`defer Stop()`;UI 设置页有启用/端口/地址,保存时 `Reload`。**文档架构(职责拆分,2026-07-08 起)**:接口规范的唯一权威=仓库根 **`接口文档.md`**(标准接口文档:术语/通道/下行报文/排版元素/两类上行(report/state)/错误码/限制/示例/修订记录),**`apidoc.html` 与它内容必须保持同步**(同一规范的 HTML 呈现,含目录锚点;改接口时二者一并更新);`功能说明.md` 为**产品/操作手册**,不承载协议细节(只留云端接入概览并指向接口文档);`测试JSON.txt` 为口语操作指南风格的样例集(刻意保留口语,只校对事实)。
- **UI `internal/ui`**(Walk,按视图/职责拆文件):
  - `App.go` 外壳(窗口 + 左导航 ListBox + 内容页切换 + 状态栏)。**主窗口 X/Alt+F4=最小化到托盘**:`mw.Closing()` 拦截(`*canceled=true`+`Hide()`,首次 notify 提示一条);托盘「退出」置 `quitting` 放行后 `walk.App().Exit(0)`(Exit 本不经 Closing,置位为保险);系统关机走 WM_QUERYENDSESSION 不发 WM_CLOSE,拦截不阻塞关机,进程被终止后云端离线由 LWT 兜底。
  - **状态监测 `StatusMonitor.go`——每台打印机一条后台持续 ping(等价 `ping -t`,独立通道)**:`syncMonitors()` 让监测与打印机列表对齐(缺的起、监测签名 `monSig`〔conn/ip/port/usbName/name〕变了停旧起新、删的停);`monitorLoop(snap, stop)` 每 `pingInterval`(1s)一次 `printsvc.Status(&snap)`(网口 `transport.QueryICMPPing`〔ICMP,不占 9100〕/ USB winspool)。**网口 30 秒离线防抖(2026-07-09 用户拍板,替代原「超时即离线」零防抖)**:用户现场一台路由带 400-500 设备偶发丢包频繁,丢一包即离线造成误报风暴——`OnlineDebounce.go` 的 `onlineDebouncer`(纯逻辑可单测,`offlineAfter` 包级 var=30s):连续失败持续满窗才判离线、任一次 ping 通立即回在线;USB 本地查询不防抖(window=0 即时生效)。**三边沿分离**:①原始成功边沿→`NudgeOnline`(联通即打不受防抖拖慢,容错窗口内恢复无 eff 边沿也秒打);②生效 label 边沿→`mw.Synchronize` 写 `printerModel.status[id]`+`refreshPrinters()`+记日志(容错窗口保持在线标签防闪烁,detail 记「网络抖动: 连续失败 Xs」;离线 detail=首败原因+已持续时长;启动即连败在未定窗显「—」);③生效布尔边沿(`lastEff` 三态)→系统通知+常驻告警窗 raise/resolve,首个生效态记基线静默(启动/监测重建〔改属性/改名〕不通知;**基线在线仍 `alertResolve`**——重建前挂起的离线告警靠它消除;**基线离线仍 `alertRaise`**——启动即离线的机器进告警窗不弹 Toast,「已持续」按程序工作时间〔首次探测失败〕起算;静默只免通知不免告警窗收尾/挂起)。**生效态(eff≥0)才写 `svc.SetPrinterOnline(id,eff==1,标签(detail))`(在线注册表,定时 `state` 上报据此合成快照;未定态 -1 不写——监测重建首轮恰逢丢包时沿用旧值,不向云端暴露单包假离线;监测本身不直接上报云端)**;`Status()` 返回后先查 `stop`(1s ping 超时可能横跨 close(stop),被停后不产出边沿动作,防删除后迟到 raise 留孤儿告警);删除打印机时 `onDeletePrinter` 显式 `alertResolve`(事件源消失,恢复边沿永不再来);`monSig` 含 Name(改名即重建监测,快照用新名)。**丢包统计 `pingStats`**(同文件,仅网口):抖动(连败≥`jitterLogMin`=5s 后恢复)INFO 一条、同台 `jitterLogGap`=60s 节流;每 `pingStatsEvery`=5 分钟一条统计(发送/丢失/连败峰值/RTT 均值),**窗口内无丢包不落日志**。`snap` 为起监测时的值拷贝(不读共享指针,避免与属性编辑竞态)。`Run` 启动 `syncMonitors()`、退出 `stopAllMonitors()`;增删/属性/云端同步后 `syncMonitors()`。**踩过的坑**:早先用"单定时器同步批量探测所有机",与真离线机(占满 1s 超时)并发会串扰、导致在线机瞬时误报离线——改成每台独立 ping 后消除。`statusInfo` 带 `detail`;`main` 启动打印机清单(含 ID)落日志。
  - `Tables.go` 两个 `TableView` 模型(`PrinterModel`/`JobModel`,embed `walk.TableModelBase`,`StyleCell` 给状态列上色,颜色 = 设计稿配色)。
  - `PrintersView.go` / `JobsView.go` / `JobPreview.go`(队列预览对话框) / `SettingsView.go` 三个视图 + 事件。
  - `AddWizard.go` 三步向导(`Dialog.Create` → `update()` 切步 → `dlg.Run()`)、`Properties.go` 属性对话框。
  - `Tray.go` 托盘(`MessageClicked` 挂 `showMainWindow`——点系统通知显主窗口)、`Icon.go` 图标。
  - **常驻告警窗 `AlertWindow.go`——右下角置顶小窗,不点不消失**(系统 Toast 停留时长由系统全局设置控制、应用无权常驻,故自绘承担持久醒目):条目按 `alertKind+key` 幂等(job-failed/jobNo、job-waiting/jobNo、printer-offline/printerID、mqtt-down),`alertRaise(kind,key,sev,text,since)`/`alertResolve` 复用 `notifyReady` 守卫+`Synchronize`(任意 goroutine 可调、快速返回);**「已持续」列动态计时**:`when`=事件起始(打印机离线=debouncer failSince 断连起点,其余=挂起时刻),`Value` 绘制时算 `time.Since`,`alertTickLoop`(alertCreate 起,1s ticker,notifyReady 退出)每秒 `PublishRowsChanged` 只重绘不 reset;离线行文本只带首因(时长交动态列,勿再凝固进字符串);自动消除=打印机恢复(含监测重建后基线在线)/打印机删除/MQTT 恢复/任务 done/重打/取消,job-failed 只能手动清(「全部知道了」/X);上限 `alertMaxItems`=200 丢最旧;**不受 `Settings.NotifyDisabled` 控制**(它管系统通知;告警窗是应用自身状态面板)。**实现坑(源码核实)**:非模态 `Dialog{FixedSize:true}.Create(nil)` 不调 `Run()`(共用主窗消息循环);**nil owner 防 `Dialog.Show` 的 `centerInOwnerWhenRun` 把窗口拉回主窗中心**(walk dialog.go:83),且不随主窗最小化被隐藏;**Create 失败须 `Dispose`+置 nil**(declarative Create 在初始化控件前就写 AssignTo,部分失败留半初始化残窗);Create 后 `SetWindowLong(GWL_EXSTYLE, |WS_EX_TOOLWINDOW|WS_EX_NOACTIVATE)` 不进任务栏/Alt-Tab+弹出不抢焦点(**点击窗内控件会正常取焦点**——NOACTIVATE 只拦系统鼠标激活,walk TableView 在 WM_LBUTTONDOWN 显式 SetFocus,属交互本义;**WS_EX_TOPMOST 位经 SetWindowLong 不生效**,MSDN:topmost 只能经 SetWindowPos 改);定位 `GetMonitorInfo(MonitorFromWindow(h,MONITOR_DEFAULTTONEAREST)).RcWork`(去任务栏工作区)右下角,每次 raise 重算位置+`SetWindowPos(HWND_TOPMOST,…,SWP_NOACTIVATE)` 保证置顶再 `Show()`(walk `Show()` 底层 SW_SHOWNA 本不激活);X 经 `Closing()` 拦截(`*canceled=true`)转清空+`Hide()` 复用不销毁;`mw.Run()` 返回后 `Dispose()`。**`showMainWindow` 须先判 `IsIconic`→`SW_RESTORE`**(walk Show=SW_SHOWNA 对最小化窗口保持最小化,SetFocus 也不还原,否则托盘/通知/告警窗点了没反应)。lxn/win 因此升直接依赖。
  - **系统通知 `Notify.go`——统一气泡方案(用户拍板)**:`App.notify(level,title,msg)` 走托盘 `tray.ShowInfo/ShowWarning/ShowError`(`Shell_NotifyIcon` NIF_INFO,walk 用 XP 兼容 V3 尺寸)——**Win7=经典气泡,Win10/11 由系统自动渲染为 Toast 进通知中心,无 OS 分支代码**;真 WinRT Toast(go-toast 起 powershell.exe / winrt-go 需高版本 Go)与 go1.20/386/单 exe 冲突,已否。任意 goroutine 可调、非阻塞(`Synchronize` 只投递,满足 printsvc 回调契约);`notifyReady`(atomic.Bool,setupTray 成功置 true、`mw.Run()` 返回置 false)守卫早期/退出后投递;开关 `Settings.NotifyDisabled` 在 UI 线程实时读。**UTF-16 截断 63/255 码元**(walk 对 `SzInfoTitle[64]`/`SzInfo[256]` 裸 copy 不保证 NUL 终止;不劈代理对,尾加「…」);msg 空用 title 兜底(szInfo 空串=撤气泡不显示)。触发:①`NotifyJobEvent`(failed=error/waiting=warn,done 不通知,本地任务也通知,main 装配);②打印机离线/恢复(StatusMonitor 在线布尔边沿);③MQTT 断线/恢复(`mqttNotifyEdge` 状态机 0中性/1连/2断,onStatus 的 Synchronize 内调用——**坑:Close/reconnect 置 cli=nil 不触发 onStatus**,中性态每次回调里靠 `mc.Active()` 判,首个状态记基线不通知,ConnectionLost 连发两次回调靠状态机去重;已知预期:「连接测试」顶号产生一对断开/恢复通知)。
  - 坑:`Dialog{}.Run(owner)` 返回 `(int, error)`;`WidgetBase.SetVisible` 无返回值。
  - **坑:对话框关闭后控件即销毁,读 `LineEdit.Text()` 会得空。** 必须在点「确定/完成」的 `OnClicked`(`dlg.Accept()` 之前)把值捕获到外层变量,再在 `Run()` 之后使用。
  - **坑:`model.PublishRowsReset()` 会清空 TableView 选中。** 刷新前记住选中项 ID,刷新后 `SetCurrentIndex` 恢复。
  - **坑:`PublishRowsReset` 行数不变时不重绘**(walk `setItemCount` 的 `LVM_SETITEMCOUNT` 带 `LVSICF_NOINVALIDATEALL`,旧文本滞留屏幕靠点击/遮挡才偶然刷新;真机现形:打印机恢复后列表一直显示「离线」,点行才更新)——**一律用 `Tables.go` 的 `publishTableRefresh`**(reset 后补发 `PublishRowsChanged(0,n-1)` 走 `LVM_REDRAWITEMS` 强制重绘),勿裸调 `PublishRowsReset`。

品牌差异全部收敛在传输层与指令层之下。

## 厂商 SDK —— 仅作参考,不作依赖

当前起步机型为 **Gprinter / 佳博**。SDK 原始包在 `F:\congmingpay\SDK\`(`C#.zip`、`Android新框架-SDK开发包.tar`),C# demo 已解压到 `F:\congmingpay\SDK\_extract_csharp\...\POSdllDemo\`。

- **本项目自研传输层,不链接厂商 DLL**(`POSDLL.dll`/`libUsbContorl.dll`),以摆脱其 32 位 x86 锁定、并天然支持多品牌(所有 ESC/POS 机器通用)。
- SDK 的价值:**ESC/POS 指令参考**(见 `佳博热敏票据打印机编程手册 v1.0.5.pdf`,本项目用票据 ESC/POS,**不用**标签 TSPL)、以及验证过的通道事实——USB 需 Windows 打印驱动(`usbprint.sys`)接管、网口即 TCP 9100。

## 构建 / 运行(已验证)

- **一次性准备并行工具链**(不动默认 Go):`go install golang.org/dl/go1.20.14@latest` → `go1.20.14 download`。
- **编译校验(快):** 用默认高版本 Go 即可 —— `go build ./...`。
- **发布构建:** `.\build.ps1` → 产出 `congmingpay.exe`(GOARCH=386、go1.20.14、`-H windowsgui`、嵌入 manifest)。已验证:PE=I386、Win 上可启动、枚举打印机、写日志。
- 运行后看 `congmingpay.log`(程序目录)排查问题。

### 已知坑(踩过,勿重蹈)

- **Walk 必须嵌入 comctl32 v6 manifest**,否则启动即崩 `TTM_ADDTOOL failed`。`build.ps1` 用 `rsrc -arch 386 -manifest app.manifest -o rsrc_windows_386.syso` 嵌入(文件名 `_windows_386.syso` 让它只在 386 链接)。
- **`x/text` 必须锁 v0.14.0**:v0.38 要求 Go 1.25,go1.20 编不了(`cannot compile Go 1.25 code`)。
- **`go.mod` 的 go 指令须为 `go 1.20`**(`go mod init` 会写成 `1.25.5`,go1.20.14 不认)。
- **`build.ps1` 保持纯 ASCII/英文**:PowerShell 5.1 把 UTF-8(无 BOM)脚本按 ANSI 解析,中文会破坏语法。
- **能否跑 Win7 取决于编译工具链版本**,不是 go.mod 那行 —— 必须用 `go1.20.14` 出包。

## 工作规则(硬性)

- **禁止臆想,必须有理有据。** 不确定的库/接口/参数/命令不得编造;以厂商 demo、手册、或代码中可验证内容为依据;无依据时标「待确定」并向用户确认。
- 标为「待确定」的项,用户拍板前不要自行补全为具体方案。
- **开发期项目,config.json 不做旧配置迁移**(用户原则:「我们就是标准」)。结构变更时说明「删除旧 config.json 重新配置」。**协议例外**:云盒兼容模式(C1–C4)见根目录 [`旧版兼容.md`](旧版兼容.md),设置开关默认开;口头「去掉旧版兼容」可先关开关再拆代码。
- **打印模板渲染两端同步**:Windows `internal/layout/` 与 Android `app/.../layout/` 的渲染/列宽/折行/放大逻辑必须行为一致。任一端改渲染,另一端同步修改并各加回归测试(注意:`Render` 层对放大超宽不报错——换行发生在打印机硬件——回归必须断言 `padCell` 输出宽度或行物理宽≤纸宽,`mustOK`/渲染成功不等于排版正确)。

## 待确定 / 下一步

- **真机验收**(需硬件):USB=先装 Windows 驱动 → 搜索/刷新 → 选中 → 打印;网口=填 IP → 打印(走 9100)。在 Win7 SP1 上复测兼容性;真机核对上报(report: accepted→done、state 定时全量 + `{"query":"printers"}` 拉取)与严格渲染的拒绝文案/错误码。
- **USB 物理在场检测(用户拍板以后再说)**:突破「关机但线插着/系统不标脱机就查不出」的 spooler 限制——SetupAPI 枚举 USBPRINT 在场设备(`SetupDiGetClassDevsEx(nil,"USBPRINT",…,DIGCF_PRESENT|DIGCF_ALLCLASSES)` + `SetupDiGetDeviceInstanceId`,端口号从 `HKLM\SYSTEM\CurrentControlSet\Enum\<id>\Device Parameters\Port Number` 读)按 `USBnnn` 端口名对应打印队列(PRINTER_INFO_2.pPortName),拔线/断电即离线;x/sys v0.20.0 已含所需 SetupDi 包装(无 DeviceInterface 系列,走 InstanceId+注册表路线),Win7 兼容。无法对应的端口回退 spooler 判定。
- **云端协议对齐**:通道=MQTT、服务端单主题一对多(上报主题如 `server`)、上行带 `merchant` 已按用户拍板实现;各消息的 type/字段名仍可与服务端协商微调(勿自行再改)。
- **Android 端**方案 —— 以后独立进行。**Android 端构建硬性规则:一律走 Android Studio IDE 构建(经 MCP `android-studio-index` 的 `ide_build_project`/`ide_diagnostics`),禁止直接跑 gradle 命令行**——Android/ 无 gradlew wrapper,且 Gradle 9.0.0 与 AGP 8.1.4 不兼容(报 `DependencyHandler.module(Object)`),命令行构建必然失败;环境细节见 `Android/CLAUDE.md` 构建节。
- 386 单架构是否满足全部目标机(如需再加 amd64:另出 `rsrc_windows_amd64.syso`)。
