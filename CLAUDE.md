# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## 项目定位

面向**餐厅厨房**的**热敏小票打印服务器**。完全独立项目,与本机其他项目(`F:\Nitebot-OTA`、`F:\dianxinxiansu` 等)无关,不复用其代码或假设。

- 目标设备:**58mm / 80mm 热敏小票打印机**,主用 **ESC/POS** 指令。
- 最终形态:**云端下发指令 → 本地服务执行 → USB/网口打印**。
- 长期目标:**多品牌**、**USB + 网口**多通道。
- 目标平台:**Windows 与 Android,两端独立开发**(不做跨平台共享核心)。Android 以后单独进行。

## 当前阶段:多打印机管理控制台

按 Claude Design 设计稿(`50-80.zip` → `_design/58-80/project/打印服务器控制台.dc.html`)用**原生 Walk** 重构为「票据打印服务器 — 管理控制台」:

- **左导航**(打印机 / 打印队列 / 系统设置)+ 内容区切换 + **底部状态栏**(`共 N 台 · N 在线 · N 任务进行中` + 临时消息)。
- **打印机视图**:工具栏(新增/刷新/测试打印/打印样票/**JSON测试**〔填 JSON 单对象或数组,本地直接走 `api.Process`,等价 MQTT 下发〕/属性/删除)+ 筛选(全部/58/80/在线)+ 搜索 + 多列 `TableView`(彩色状态列)。
- **打印队列视图**:任务表(重新打印/取消/清除已完成),状态彩色。
- **系统设置**:服务名(**用作窗口标题与托盘提示**,保存即时生效、空回退默认) + **云端 MQTT**(broker/端口/用户/密码/短商户号/**上报主题**——**启用时全部必填**,任一缺失/非法拦截保存) + **本地接口文档服务**(启用/端口/局域网地址,只读文档) + 保存。
- **新增打印机三步向导** + **属性对话框**。
- **多打印机 + 并发打印**:每任务独立 goroutine;**离线转「等待重试」队列、联通即打**(唯一自动重试,仅连接类、不限次数);刷新时并发查所有打印机状态。

设计稿是自定义样式 web mock,原生 Walk 还原**结构/交互/彩色状态**,视觉是标准 Win32(非 mock 的浅蓝 Fluent/圆角/开关)。

> 云端通道 = **MQTT(唯一,已实现,`internal/mqtt`)**:订阅「聪明付短商户号」主题收打印(payload=`PrintRequest` 单/数组),向**配置的「上报主题」**(`Settings.MQTT.ReportTopic`;**服务端单主题一对多**——所有客户端发同一主题如 `server`,旧配置自动迁移为 `<短商户号>/report`)发布五类上行:受理回执 `ack`、**打印结果 `result`**(任务终态成功/失败)、**打印机列表 `printerList`**(全量快照含每台配置参数;触发=连接基线/云端登记覆盖/UI 增删改/JSON测试变更)、**打印机在线离线 `printerStatus`**(数组承载多设备,边沿即报)、APP 上下线 `status`。**每条上行均带 `merchant`(短商户号)字段标识来源**。明文 + 用户名密码;发布 best-effort 不补发、失败原因落日志;**数据 HTTP 通道已移除**(唯一数据通道=MQTT);另有 `internal/docserver` 提供**只读接口文档** HTTP 页(仅文档、不做数据)。上行消息 type/字段为本 APP 约定,可与云端协商调整。

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

## 架构(分层,为多品牌/多通道/云端铺路)

- **传输层 `internal/transport`**:接口 `Printer{ Open/Write/Close }` + `Print(p, data)` 便捷封装。
  - `Spooler.go`(`//go:build windows`,**USB/驱动**):经 `github.com/alexbrainman/printer` 走 winspool —— `ReadNames` 列已装打印机、`Open`+`StartRawDocument`(自动选 RAW/XPS_PASS)+`Write`+`EndDocument` 发**原始字节**。`ListRealPrinters()`(过滤虚拟打印机)供 UI 枚举。
  - `Tcp.go`(**网口**):`net.DialTimeout("tcp", ip+":9100")` 直发。`RawPrintPort=9100`。**拨号失败分两类**:`isConnRefused`(RST=主机在但 9100 关/未就绪/被占)→ `ConnError{Refused:true}`;其余(超时/不可达)→ 普通 `ConnError`。〔**踩过的坑**:Windows 上 `errors.Is(err, syscall.ECONNREFUSED)` 命不中——`syscall.ECONNREFUSED` 在 Windows 是另一 sentinel,底层实际是 `syscall.Errno==10061`(WSAECONNREFUSED);故 `isConnRefused` 既比 `ECONNREFUSED`(Linux 111)也比数值 10061〕。`transport.IsRefused(err)` 供 printsvc 区分**占用短退避**(RST)与**离线退避**(超时)两档等待重试。
  - `Status.go` / `SpoolerStatus.go` / `Ping.go`(**在线/状态检测**):**高频巡检(1s)网口走 `Ping.go` 的 `QueryICMPPing`——Windows `IcmpSendEcho`(iphlpapi.dll,普通权限,不碰 9100),ping 通=在线**;USB 走 winspool `GetPrinter`(经 `golang.org/x/sys/windows` 直调)。**4000 端口的缺纸/开盖详细状态检测(网口 `ESC v` 走端口 4000)已按用户要求彻底移除**(原 `QueryTCPStatus`/`StatusPort`/`parseGprinterStatus` 及 `PaperNearEnd`/`Raw` 字段,连同死代码 `QueryTCPOnline` 一并删除)——网口只有 ICMP 在线/离线;USB 的 winspool 状态位(缺纸/开盖)保留仅供 UI 状态列展示,普通 RAW 驱动多单向、物理缺纸/开盖未必上报,**不上报云端**。
- **指令层 `internal/escpos`**:`Builder`(链式)构建 ESC/POS——`1B 40` 初始化、`1B 61 n` 对齐、`1B 45 n` 加粗、`1D 21 n` 倍宽高、`1D 56 01` 半切、`0A` 换行;**中文经 `util.ToGB18030` 编码**。`BuildTestReceipt(now, widthMM)` 生成测试小票(按纸宽自适应)。**纸宽自适应 `Layout.go`**:`CharsPerLine`—58mm=384点=32字符、80mm=576点=48字符(据佳博 demo/手册,标准行模式无需设宽指令)。**重印抬头 `Reprint.go`**:`ReprintBanner(widthMM)` 生成醒目「重打小票/打印服务器重印」,由 `printsvc` 在 `entry.reprintNext`(手动「重新打印」或消息 `reprint:1`)时前置到小票。蜂鸣见 `Buzzer.go`:`BuildBuzzer`=`ESC B n t`(手册命令62);报警灯扩展(`ESC C`,命令63)未接入、代码已清理,需要时按手册实现。蜂鸣属厂商扩展指令,多品牌接入时需适配。二维码 `QRCode.go`=`GS ( K` Model 2;`SetSize`(GS ! 放大)、`Barcode.go`(CODE128/CODE39 = GS k + GS h/w/H)、`Raster.go`(图像→单色位图 GS v 0,含 Floyd-Steinberg 抖动)、`Raw`(嵌原始字节)、`DisplayWidth`(中文=2 列宽)、`WidthDots`(58→384/80→576)。
- **JSON 排版渲染 `internal/layout`**:`Render(contents, widthMM)` 把云端 **MQTT type=5 的 JSON 小票排版数组**渲染为 ESC/POS。**全部严格模式(用户拍板)**:字段缺失=文档声明默认(合法);字段存在但非法 → **整单拒绝**,error 带 `contents[i](种类)` 定位(深层函数只报原因,`Render` 主循环统一包前缀),ack ok:false + 落日志,不产出部分字节。合法值:size=两位 0-7、align=left/center/right、表格列宽 1-100 整数**总和恰 100**、行单元格数=列数、单元格仅标量、both_sides 恰 2 段、条码 size ∈{"","22","33"}·hri 0-3、png 须 http(s)/dataURI 且下载解码成功(非 200 拒)、未知元素 type 拒、有 tbody 无 thead 拒;`cut` 元素按文档跳过;**qrcode 的 size 例外:不拒,`logger.Warnf` 每次渲染至多一条**。支持元素:`title`/`text`/`div_line`·`div_star`/表格/`both_sides`/`qrcode`(单码 native、双码位图合成)/`bc128`·`code39`/`png`/`cut`。`Sample.go` 内嵌样例(**png 为内嵌 data URI**——严格模式下外网 URL 断网会整单拒印),UI「打印样票」验证;`Render_test.go` 锁定全部拒绝点 + 样例(含 测试JSON.txt fixture)必须通过。依赖 `github.com/skip2/go-qrcode`(仅双码位图)。
- **消息处理核心 `internal/api`**(无 HTTP,只留复用逻辑):`Process(cfg,svc,*PrintRequest) (jobNo, changed, err)` + `ParseRequests(raw) ([]PrintRequest, wasArray, err)` + `resolveTarget` + 请求模型(`PrintRequest`/`PrinterRef`)。供 **MQTT** 与 UI「JSON测试」复用。**HTTP 数据传输已移除**(原 `Server.go`/`Swagger.go`/`Printers.go`/`/api/*`/`/swagger` 删除,build.ps1 去 swag 步骤)——现只有 `internal/docserver` 的**只读文档** HTTP 页,不含任何数据/打印端点。
  - **打印消息(`PrintRequest`)**:**目标二选一** `printer`(**多态 `PrinterRef`**:字符串=名/ID,或对象 `{name,ip,brand,width}`)**或 `gateway`**〔网口 IP;或 `"usb"`=首台 USB 机〕、id(仅回执/日志对应,**不去重**)、type〔5=JSON排版/1=ESC base64〕、pWidth、**pCopy**、contents;**必填字段(无降级,缺一即报错、不回退打印机默认):`buzzer`(0关1开)、`cut`(0关1开)、`reprint`(0=普通/1=带醒目「重打」抬头)、`headLines`、`tailLines`(0=默认;**范围 0-100,超出拒**)**;`pWidth` 填了非 58/80 也拒(不填合法)——`req.validate()` 校验后构建每台 `Options`;**且 `config.UpdatePrinterFromCloud` 把 5 参数 + printer 身份(name/brand/width)覆盖并持久化到目标打印机(cloud authoritative,属性页同步),打印机属性默认仅供本地「测试打印/打印样票」**。**payload 可为单对象或对象数组**——`ParseRequests` 按首字节 `[`/`{` 分流;数组=一次多条,**逐条 `Process`(不去重,每条都打)**,各任务 `Submit` 后各自并发 `dispatch`。
  - **随打印自动登记打印机(建列表)**:`resolveTarget` 目标**带 IP** 且**未注册** → `config.UpsertPrinter` 自动新增入库;**登记必须能确定纸宽:`printer.width`→`pWidth` 回退链,两者皆缺/非法即拒(不再兜底 80)**。`Process` 返回 `registered||changed`,调用方(MQTT/JSON测试)据此 `save`+刷新列表+**上报 printerList**。已注册机也被打印消息参数+身份覆盖并持久化。USB 与"仅名/ID"目标不自动登记。
  - **打印分支流程**:①带目标 → 未注册先自动登记 → `svc.Submit` 提交,`dispatch` 连接打印:在线即打、离线转「等待重试」队列(在线后自动打)。②无目标 → `Process` 报「未指定打印目标…」,调用方 `logger.Errorf` 落日志。
- **云端通道 `internal/mqtt`(唯一)**:`Client`(paho.mqtt.golang,MQTT 3.1.1)持 cfg/svc/cfgPath/onChange/onStatus(**id 去重已移除,收到即打**;`onPrint` 逐条 `Process` + 详细日志)。`Start/Reload(Settings.MQTT)/Close`;明文 + 用户名密码;**订阅 `<短商户号>`** 收打印 → `ParseRequests`→`Process`→回执;**发布到配置的「上报主题」(`ReportTopic`,与 broker/短商户号同级必要——启用而缺任一则只停不连;LWT 也发此主题;设置页启用时六项全必填拦截保存)**,五类上行且**每条都带 `merchant` 字段(短商户号,服务端单主题一对多靠它分辨来源)**:受理回执 `{type:ack,merchant,id〔恒回显,无 omitempty〕,ok,jobNo,message}` + **打印结果 `{type:result,merchant,id,jobNo,ok,printer,message,ts}`(`Report.go`,由 printsvc 终态事件驱动,见下)** + **打印机列表 `{type:printerList,merchant,printers:[{printerId,printer,brand,width,conn,ip/port 或 usbName,buzzer,cut,headLines,tailLines,lastPrint}],ts}`(`Report.go` `PublishPrinterList`,全量快照幂等、不含在线态;数据全取自 cfg,触发=OnConnect 基线/onPrint changed/UI 增删改属性〔`App.publishPrinterList`〕/JSON测试变更;仅改 LastPrint 的测试打印不触发)** + **打印机在线离线 `{type:printerStatus,merchant,printers:[…],ts}`(`Report.go`,数组承载多设备,StatusMonitor 在线布尔边沿触发,含首查基线;MQTT 未启用时静默跳过)** + APP 上下线 `{type:status,merchant,event:online/offline}`(online 在 OnConnect 发、offline 用 LWT 遗嘱 + Close 主动发)。**`publish` best-effort 但有据可查**:不排队不补发;未连接/主题空时若启用则 `logger.Errorf`,发布后异步查 token、失败 `logger.Errorf`(网络异常正常发送、日志输出失败原因——用户需求)。〔**踩过的坑**:paho v1.4.3 在 ConnectRetry 下「连接中」`IsConnected()` 也为 true,该窗口 QoS1 Publish 只进内部存储、token 永不完成,CleanSession 连上后 `persist.Reset()` 静默丢弃 + `tok.Wait()` goroutine 永久泄漏——故 publish 必须判 **`IsConnectionOpen()`**(仅真连上才 true)转「跳过+日志」,残余瞬断窗口用 **`WaitTimeout(30s)`** 兜底〕。**自订阅回环守卫**:上报主题 == 订阅主题(短商户号)会让 ack 被自己收到→解析失败→再发失败 ack 无限风暴——设置页拦截保存 + `reconnect` 拒连双层防护。`SetAutoReconnect`+`SetConnectRetry` 自动重连(重订阅在 `OnConnect` 内,重连自动恢复;订阅+上线 publish 放 goroutine 里做,不阻塞 paho 回调线程);`TestConnect` 供设置页「连接测试」;`SanitizeMerchant` 净化短商户号(仅 `[A-Za-z0-9_]`;上报主题不走净化,允许 `/`、禁通配符 `+`/`#`,设置页保存时校验)。**旧配置迁移**:`main` 启动时「有短商户号、无上报主题」→ 自动补 `<短商户号>/report` 并落盘。**ClientID=`cmp-<商户号>-<进程随机4字节hex>`**〔`New` 里 `randSuffix()` 生成一次;**防同商户号多客户端(云端工具/另一实例)互相顶号**——顶号表现正是「用一会就不收消息」〕。**`SetOnStatus` 状态回调**:`setConnected`/`setErr` 触发 `fireStatus`,UI marshal 回界面线程刷新 MQTT 状态标签(解决「保存后卡在连接中…」——连接是异步的,靠回调而非保存时那一次即时读)。`main` 建 `mqtt.New(cfg,svc,cfgPath).Start()`,`defer Close()`;UI 在 `Run` 设 `onChange`+`onStatus`、设置保存时 `Reload`。**库版本**:`paho.mqtt.golang v1.4.3`(依赖仅 gorilla/websocket + x/sync,**不顶高 x/text v0.14/go1.20/Win7**;MQTT5 的 `paho.golang` 需 Go1.23+x/text0.21 **不可用**)。
- **数据模型 `internal/model`**:`Printer`(名称/**品牌**〔佳博/飞蛾/其他,佳博飞蛾同 ESC/POS 协议〕/规格/连接/IP·端口/USB驱动名/**蜂鸣开关** `BuzzerEnabled`/**首尾空行** `HeadLines`(绝对)·`TailLines`(尾部**偏移**,实际=`escpos.TailFeed(width,offset)`=基数+偏移,**基数按纸宽:58=5、80=3**,偏移可负、下限0)〔走纸 LF,属性页设,标签动态显示基数〕/**切刀** `CutDisabled`(反向,`Cuts()`))、`Job`(任务+状态,状态含 `JobWaiting`「等待重试」,`Active()` 纳入)、`Settings`(服务名/MQTT)。品牌当前仅元数据+未来适配预留,不分支打印逻辑。**每台重试字段(NoRetry/RetryMax)已移除**——重打逻辑只有「离线等待联通即打」+「手动/消息 `reprint` 抬头」,不再有每台有限重试。
- **打印服务 `internal/printsvc`**:`Service.Submit` 每任务开 goroutine → **并发打印**;`Retry`/`Cancel`/`ClearDone`;`Submit(p,doc,data,*Options)` 的 `Options`(cut/buzzer/head/tail/**reprint** 指针/**`CloudID *uint32`**〔云端消息 id 随任务走,`api.Process` 填 `&req.ID`,nil=本地任务〕)**按需覆盖每台默认**(nil=默认),供 MQTT/JSON测试传参(`reprint` 是每单标记,不写回打印机);**终态事件 `Events.go`**:`SetOnJobFinal(func(JobFinalEvent))`——`setStatus` 到 `JobDone/JobFailed` 时锁内快照、锁外触发(四个终态出口全经 setStatus 收口各恰一次;`JobWaiting` 不触发),`main` 装配转 `mc.PublishJobResult`(CloudID==nil 的本地任务过滤不上报),回调契约:快速返回不阻塞;`Status(p)` 是**唯一公共在线/离线检测入口**(网口 ICMP 不碰 9100 / USB winspool),监测、dispatch 判在线、新机监测三处共用(见下)。`dispatch` **统一组装收尾**:蜂鸣 → 重印抬头 → `FeedBytes(HeadLines)` → 内容 → `Finish(Width,TailLines,Cuts())`(尾部走纸 + 按切刀设置切纸)。**内容构建(`BuildTestReceipt`、`layout`)不再自带走纸/切纸**——避免切纸前走纸不足把底部内容切断,且每台「尾部空行/切刀」设置对测试小票与云端样票一致生效。`layout` 的 JSON `cut` 元素被跳过(收尾统一)。`notify` 回调由 UI marshal 回界面线程。
  - **dispatch 状态机(判在线分支 + 真打裁决 + 两档退避)**:① **首次派发**先 `Status(&e.printer)` 判在线(仅网口 ICMP,不碰 9100;`entry.gated` 只门控一次)——离线则直接进 `JobWaiting` 省 5s 空拨号;**重试一律直接真打**(真打为最终裁决),避免 ICMP 被过滤但 9100 可打的机被假离线永久排队。USB 不做 ICMP 门控(交 spooler 排队)。② **真打**(`transport.Print` 一次性 Open→Write→Close)按错误分类:`err==nil`→`JobDone`;`transport.IsRefused`(RST=被占/端口未就绪)→ `JobWaiting`「**占用短退避**」(1s→2s→3s);`transport.IsConnError` 非 refused(超时/不可达)→ `JobWaiting`「**离线退避**」(5s→30s);其余(已连上但 Write 失败)→ `JobFailed`(避免重复出纸)。**RST 不再快速失败**(旧 `printerBusy` 快速失败已删)——被占/端口未就绪都排队等待,联通/释放即打。
    - **每台打印串行锁**(`Service.printLocks` = `printerID→*sync.Mutex`):**仅在真打那步持锁**(退避 sleep 不持锁),本进程对同一台同时只开一个 9100 连接——`pCopy` 多份/并发任务**串行**打印、彼此不自撞 RST;不同打印机仍并行。有此锁后自撞不可能,故 RST 必是**第三方占用或端口未就绪**,统一排队等待(`printerBusy`/`busy-guard` 已随快速失败一并删除)。
    - **坏端口/错IP 可见告警(不静默丢单)**:占用流连续被拒超 `longWaitWarn`(20s)→ 把 `job.Err` 置「长期等待·疑似端口未就绪/故障」并**节流** `logger.Errorf` 一次;任务**保持 `JobWaiting` 永不自动失败**。UI 打印队列「详情」列展示该告警(红色,`Tables.go` `JobModel` col 5——份数列已删,列号:任务号0/文档1/打印机2/状态3/时间4/详情5)。
  - **上线/释放即打**:等待中的退避 `select` 同时监听 `entry.wake`;UI 每台持续 ping 检测到某机**上线**(离线→就绪)即调 `Service.NudgeOnline(printerID)` 唤醒它所有 `JobWaiting` 任务、重置退避**立即重打**(~1s)。离线走此路径秒打;**占用释放**(ICMP 全程在线、无边沿,不触发 nudge)靠**占用短退避**(~1-3s)再打抓取——这是「空闲零接触 9100」下的取舍(真「秒级」需探 9100,已被用户否)。即"打印在在线/离线判定之后:不通/被占进队列,通了/释放了就打"。
  - **「重打」抬头两种触发**:`entry.reprintNext` 决定 `dispatch` 是否前置 `ReprintBanner`——**① 用户点「重新打印」(`Retry`)时置位;② 打印消息 `reprint:1`(经 `Options.Reprint`)首次派发即置位**。连接等待重试不改此位(联通即打时若原为 reprint:1 仍带抬头,否则不带)。
- **配置 `internal/config`**:本地 JSON(与 exe 同目录 `config.json`)—— `Settings` + `Printers[]`(持久化打印机注册表)。**打印机 ID 用 `NewPrinterID()`=时间戳+原子递增序号**〔**踩过的坑**:原来用裸 `time.Now().UnixNano()`,Windows 时钟精度粗(~100ns),数组一次登记多台时并发两次调用撞出**相同 ID**,状态表按 `p.ID` 存取导致两台状态互相绑定(关一台另一台跟着离线);FindPrinter/RemovePrinter 也会误取。**AddWizard/UpsertPrinter 一律走 `NewPrinterID`,勿再用裸 UnixNano**〕。`HealPrinterIDs()` 启动时自愈旧配置里空/重复 ID(`main` 加载后调用、有改动即落盘)。**config.json 损坏不再静默丢弃**:`main` 把坏文件备份为 `config.json.bad-<时间戳>` 并落日志,再用默认配置启动(打印机注册表可从备份手工恢复)。
- **日志 `internal/logger`**:全局文件日志(与 exe 同目录 `congmingpay.log`)。**业务代码一律调用 `logger.Info/Infof/Warn/Warnf/Error/Errorf`,不直接用标准库 `log`**;`main` 启动时 `logger.Init(path)`。GUI 无控制台,靠此排查。
- **工具 `internal/util`**:`ToGB18030`(x/text 编码)。
- **资源 `internal/assets`**:`//go:embed app.png` 图标,保证单 exe。
- **接口文档服务 `internal/docserver`(仅局域网、只读)**:标准库 `net/http`,`//go:embed apidoc.html`。**只暴露 `GET /` 返回内嵌接口文档页;其它路径 404、其它方法 405;无任何打印/数据端点**(数据只走 MQTT,不越权)。`Server.Start/Reload/Stop(model.DocServer)`;`LANIP()` 取私网 IPv4 供 UI 展示访问地址。配置 `Settings.DocServer{Enabled,Port}`(默认启用、8080;`main` 对无此段的旧配置迁移为默认并落盘)。`main` 建 `docserver.New().Start()`、`defer Stop()`;UI 设置页有启用/端口/地址,保存时 `Reload`。**`apidoc.html` 与 `功能说明.md` 内容需保持同步**(二者同为接口规范的呈现,改接口时一并更新)。
- **UI `internal/ui`**(Walk,按视图/职责拆文件):
  - `App.go` 外壳(窗口 + 左导航 ListBox + 内容页切换 + 状态栏)。
  - **状态监测 `StatusMonitor.go`——每台打印机一条后台持续 ping(等价 `ping -t`,独立通道)**:`syncMonitors()` 让监测与打印机列表对齐(缺的起、连接身份 `monSig`〔conn/ip/port/usbName〕变了停旧起新、删的停);`monitorLoop(snap, stop)` 每 `pingInterval`(1s)一次 `printsvc.Status(&snap)`(网口 `transport.QueryICMPPing`〔ICMP,不占 9100〕/ USB winspool),**不做防抖——超时即离线、通即在线**,仅状态标签变化时 `mw.Synchronize` 写 `printerModel.status[id]`+`refreshPrinters()`+记日志 `打印机状态: [id=…] 名称『…』addr → 标签(detail)`。**另按在线布尔边沿(`lastOnline`,含首查基线)调 `mc.PublishPrinterStatus` 上报云端**——布尔边沿而非标签边沿(「就绪↔缺纸」标签变但在线态不变,不上报),在监测 goroutine 内直发(paho 线程安全),不进 UI 线程。`snap` 为起监测时的值拷贝(不读共享指针,避免与属性编辑竞态)。`Run` 启动 `syncMonitors()`、退出 `stopAllMonitors()`;增删/属性/云端同步后 `syncMonitors()`。**踩过的坑**:早先用"单定时器同步批量探测所有机",与真离线机(占满 1s 超时)并发会串扰、导致在线机瞬时误报离线——改成每台独立 ping 后消除。`statusInfo` 带 `detail`;`main` 启动打印机清单(含 ID)落日志。
  - `Tables.go` 两个 `TableView` 模型(`PrinterModel`/`JobModel`,embed `walk.TableModelBase`,`StyleCell` 给状态列上色,颜色 = 设计稿配色)。
  - `PrintersView.go` / `JobsView.go` / `SettingsView.go` 三个视图 + 事件。
  - `AddWizard.go` 三步向导(`Dialog.Create` → `update()` 切步 → `dlg.Run()`)、`Properties.go` 属性对话框。
  - `Tray.go` 托盘、`Icon.go` 图标。
  - 坑:`Dialog{}.Run(owner)` 返回 `(int, error)`;`WidgetBase.SetVisible` 无返回值。
  - **坑:对话框关闭后控件即销毁,读 `LineEdit.Text()` 会得空。** 必须在点「确定/完成」的 `OnClicked`(`dlg.Accept()` 之前)把值捕获到外层变量,再在 `Run()` 之后使用。
  - **坑:`model.PublishRowsReset()` 会清空 TableView 选中。** 刷新前记住选中项 ID,刷新后 `SetCurrentIndex` 恢复。

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

## 待确定 / 下一步

- **真机验收**(需硬件):USB=先装 Windows 驱动 → 搜索/刷新 → 选中 → 打印;网口=填 IP → 打印(走 9100)。在 Win7 SP1 上复测兼容性;真机核对上报(ack→result、printerStatus 边沿、printerList)与严格渲染的拒绝文案。
- **云端协议对齐**:通道=MQTT、服务端单主题一对多(上报主题如 `server`)、上行带 `merchant` 已按用户拍板实现;各消息的 type/字段名仍可与服务端协商微调(勿自行再改)。
- **Android 端**方案 —— 以后独立进行。
- 386 单架构是否满足全部目标机(如需再加 amd64:另出 `rsrc_windows_amd64.syso`)。
