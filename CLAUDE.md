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
- **打印机视图**:工具栏(新增/刷新/测试打印/打印样票/**JSON测试**〔填 JSON 单对象或数组,暂代 MQTT 直接打印,走 `api.Process`〕/属性/删除)+ 筛选(全部/58/80/在线)+ 搜索 + 多列 `TableView`(彩色状态列)。
- **打印队列视图**:任务表(重新打印/取消/清除已完成),状态彩色。
- **系统设置**:服务名/监听端口/默认纸张/失败自动重印 + **云端 MQTT**(broker/端口/用户/密码/主题) + 保存。
- **新增打印机三步向导** + **属性对话框**。
- **多打印机 + 并发打印**:每任务独立 goroutine;**失败按设置自动重印一次**;刷新时并发查所有打印机状态。

设计稿是自定义样式 web mock,原生 Walk 还原**结构/交互/彩色状态**,视觉是标准 Win32(非 mock 的浅蓝 Fluent/圆角/开关)。

> 云端 MQTT 目前仅在设置里存参数,不实际连接。**暂用本地 HTTP API 代 MQTT**(打印下发 `POST /api/print`、打印机清单下发 `POST /api/printers`),同一 `PrintRequest`/`Process`/`UpsertPrinter` 将来供 MQTT 复用。真实 MQTT 消息格式(`type=5`、`gateway`、`contents`、`id`、`pWidth`、`pCopy`)见用户提供的示例,**其余字段勿臆测**。

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
  - `Spooler.go`(`//go:build windows`,**USB/驱动**):经 `github.com/alexbrainman/printer` 走 winspool —— `ReadNames` 列已装打印机、`Open`+`StartRawDocument`(自动选 RAW/XPS_PASS)+`Write`+`EndDocument` 发**原始字节**。`ListPrinters()` 供 UI 枚举。
  - `Tcp.go`(**网口**):`net.DialTimeout("tcp", ip+":9100")` 直发。`RawPrintPort=9100`。
  - `Status.go` / `SpoolerStatus.go` / `Ping.go`(**在线/状态检测**):**高频巡检(1s)网口走 `Ping.go` 的 `QueryICMPPing`——Windows `IcmpSendEcho`(iphlpapi.dll,普通权限,不碰 9100),ping 通=在线**;USB 走 winspool `GetPrinter`(经 `golang.org/x/sys/windows` 直调)。详细状态 `QueryTCPStatus`(网口 `ESC v`=`1B 76` **走端口 4000**〔非 9100!手册命令64〕读回 4 字节缺纸/开盖/错误,失败回退测 9100)**保留备用、当前不高频调用**——纯 ping 只给在线/离线,读不到缺纸/开盖。USB 普通 RAW 驱动多单向,物理缺纸/开盖未必上报。
- **指令层 `internal/escpos`**:`Builder`(链式)构建 ESC/POS——`1B 40` 初始化、`1B 61 n` 对齐、`1B 45 n` 加粗、`1D 21 n` 倍宽高、`1D 56 01` 半切、`0A` 换行;**中文经 `util.ToGB18030` 编码**。`BuildTestReceipt(now, widthMM)` 生成测试小票(按纸宽自适应)。**纸宽自适应 `Layout.go`**:`CharsPerLine`—58mm=384点=32字符、80mm=576点=48字符(据佳博 demo/手册,标准行模式无需设宽指令)。**重印抬头 `Reprint.go`**:`ReprintBanner(widthMM)` 生成醒目「重打小票/打印服务器重印」,由 `printsvc` 在重印(attempts>1)时前置到小票。蜂鸣见 `Buzzer.go`:`BuildBuzzer`=`ESC B n t`(手册命令62)、`BuildBuzzerAndAlarm`=`ESC C m t n`(命令63,报警灯机型)。蜂鸣属厂商扩展指令,多品牌接入时需适配。二维码 `QRCode.go`=`GS ( K` Model 2;`SetSize`(GS ! 放大)、`Barcode.go`(CODE128/CODE39 = GS k + GS h/w/H)、`Raster.go`(图像→单色位图 GS v 0,含 Floyd-Steinberg 抖动)、`Raw`(嵌原始字节)、`DisplayWidth`(中文=2 列宽)、`WidthDots`(58→384/80→576)。
- **JSON 排版渲染 `internal/layout`**:`Render(contents, widthMM)` 把云端 **MQTT type=5 的 JSON 小票排版数组**渲染为 ESC/POS(将来 MQTT 直接调用)。支持元素:`title`/`text`(bold/size"WH"/align)/`div_line`·`div_star`(带标题分割线)/表格(`thead` 对象保序或数组 + `tbody`,列宽%、`line_div`/`line_space`/`size`)/`both_sides`/`qrcode`(单码 native、双码位图合成)/`bc128`·`code39`/`png`(下载/base64→光栅)/`cut`。`Sample.go` 内嵌样例,UI「打印样票」按钮验证。依赖 `github.com/skip2/go-qrcode`(仅双码位图)。`size` 等未尽述处按合理映射,真机微调。
- **本地 HTTP API `internal/api`**(暂代 MQTT):`main` 建**单个** `Server`(`NewServer(cfg,svc,cfgPath)`)同时交给 `startAPI` 与 `ui.NewApp`;起 goroutine 监听 `Settings.APIPort`(默认 8080)。
  - **`POST /api/print`**(`PrintRequest`:**目标二选一** `printer`(**多态 `PrinterRef`**:字符串=名/ID,或对象 `{name,ip,brand,width}`)**或 `gateway`**〔网口 IP;或 `"usb"`=首台 USB 机〕、id 防重打、type〔5=JSON排版/1=ESC base64〕、pWidth、**pCopy** 份数、contents;**必填字段(无降级,缺一即报错、不回退打印机默认):`buzzer`(0关1开)、`cut`(0关1开)、`retry`(重试次数 0=不重试/N=重试N次)、`headLines`、`tailLines`(0=默认)**——`Process` 里 `req.validate()` 校验后由这些值构建每台 `Options`;**且 `config.UpdatePrinterFromCloud` 把这 5 参数 + printer 身份(name/brand/width)覆盖并持久化到目标打印机(cloud authoritative,属性页同步),打印机属性默认仅供本地「测试打印/打印样票」)**。**body 可为单对象或对象数组**——`ParseRequests` 按首字节 `[`/`{` 分流;数组=一次多台,逐 id 防重后**并发 `Process`**,返回**逐台 `PrintResponse` 数组**(单对象仍返回单响应,兼容旧调用方)。`Process`/`resolveTarget` 供 UI「JSON测试」与将来 MQTT 复用。
  - **随打印自动登记打印机(建列表)**:`resolveTarget` 目标**带 IP**(`printer.ip` 或 `gateway` 是 IP)且**未注册** → `config.UpsertPrinter` **自动新增入库**(name=`printer.name` 否则 IP、brand 空→其他、width=`printer.width`/pWidth/80),`Process` 返回 `registered||changed`(新登记或参数被云端覆盖);`handlePrint`/UI「JSON测试」据此 `save`+刷新列表。**已注册的机也会被打印消息的参数+身份覆盖并持久化(cloud authoritative,见 `UpdatePrinterFromCloud`)**——不再是"只用不改"。USB 与"仅名/ID"目标不自动登记(但仍被参数覆盖)。取代了旧的"未注册临时直连不落库"。
  - **打印分支流程**:①带目标 → 未注册先自动登记 → `svc.Submit` 提交,`dispatch` 连接打印:在线即打、离线转「等待重试」队列(在线后自动打);已注册的直接提交、不预检(靠 UI 后台每 15s 持续检测状态)。②无目标(printer/gateway 全空)→ `Process` 报「未指定打印目标…」,`handlePrint` 经 `logger.Errorf("打印请求失败: …")` **落日志**。首次打印不做单独"在线预检",连接尝试本身即在线测试。
  - **`/api/printers`**:`GET` 列表;**`POST` = 云端下发打印机清单**(`[]PrinterSync{conn:"ip"|"usb",ip,port,usbName,name,brand,width}`)——`config.UpsertPrinter` **按身份增量 upsert**(网口按 IP、USB 按 USBName;命中只改展示字段、**保留本地个性化设置**,未命中新建;brand 空→其他、width 默认 80),**替代手动创建**;save 后经 `Server.onChange` 通知 UI 刷新。
  - `GET /swagger/` UI + `/swagger/doc.json`。**Swagger:swag CLI 从注解生成 `docs/swagger.json`(见 build.ps1),但不引 swag 运行时库(会顶高 x/text 破坏 go1.20)——`Swagger.go` 内嵌 json + CDN swagger-ui。config 加 `sync.RWMutex`(`AddPrinter/UpsertPrinter/FindPrinter/PrinterList/RemovePrinter`)保 API 与 UI 并发。**
- **数据模型 `internal/model`**:`Printer`(名称/**品牌**〔佳博/飞蛾/其他,佳博飞蛾同 ESC/POS 协议〕/规格/连接/IP·端口/USB驱动名/**蜂鸣开关** `BuzzerEnabled`/**首尾空行** `HeadLines`(绝对)·`TailLines`(尾部**偏移**,实际=`escpos.TailFeed(width,offset)`=基数+偏移,**基数按纸宽:58=5、80=3**,偏移可负、下限0)〔走纸 LF,属性页设,标签动态显示基数〕/**切刀** `CutDisabled`(反向,`Cuts()`)/**重打** `NoRetry`(反向,`Retries()`)+`RetryMax`(`MaxRetries()`,≤0→1))、`Job`(任务+状态,状态含 `JobWaiting`「等待重试」,`Active()` 纳入)、`Settings`(服务名/端口/默认纸张/自动重印/MQTT)。品牌当前仅元数据+未来适配预留,不分支打印逻辑。
- **打印服务 `internal/printsvc`**:`Service.Submit` 每任务开 goroutine → **并发打印**;`Retry`/`Cancel`/`ClearDone`;失败按**每台 `Retries()`/`MaxRetries()` 重打**;`Submit(p,doc,data,*Options)` 的 `Options`(cut/buzzer/head/tail/retry/retryMax 指针)**按需覆盖每台默认**(nil=默认),供 API/MQTT 传参;`Status(p)` 查单机状态(供 UI 并发刷新)。`dispatch` **统一组装收尾**:蜂鸣 → 重印抬头 → `FeedBytes(HeadLines)` → 内容 → `Finish(Width,TailLines,Cuts())`(尾部走纸 + 按切刀设置切纸)。**内容构建(`BuildTestReceipt`、`layout`)不再自带走纸/切纸**——避免切纸前走纸不足把底部内容切断,且每台「尾部空行/切刀」设置对测试小票与云端样票一致生效。`layout` 的 JSON `cut` 元素被跳过(收尾统一)。`notify` 回调由 UI marshal 回界面线程。
  - **网络异常等待重试**:失败若为 `transport.IsConnError`(网口拨号失败/超时,`Tcp.go` 的 `Open` 包成 `transport.ConnError`)→ 转 `JobWaiting`「等待重试」,**退避(5s 起、×2 至 30s 上限)后持续重试、不计 `retryMax`**,堆在任务列表直到网络恢复打成功或被 `Cancel`(置 `entry.cancelled`,休眠 goroutine 醒来即停)。非连接错误仍按 `retryMax` 有限重试。
  - **上线即打(监测联动)**:等待中的退避 `select` 同时监听 `entry.wake`;UI 每台持续 ping 检测到某机**上线**(离线→就绪)即调 `Service.NudgeOnline(printerID)` 唤醒它所有 `JobWaiting` 任务、重置退避**立即重打**(~1s),不用干等退避。退避是兜底(监测漏了也能重试),监测是加速(打印机一恢复就秒打)。即"打印在在线/离线状态筛选之后:不通进队列、通了就打"。
  - **「重打」抬头只在手动重打时加**:`entry.reprintNext` 决定 `dispatch` 是否前置 `ReprintBanner`——**仅用户点「重新打印」(`Retry`)时置位**;**自动重试(连接等待重试 与 retryMax 有限重试)都不置**(自动重试对用户透明,不标重打)。
- **配置 `internal/config`**:本地 JSON(与 exe 同目录 `config.json`)—— `Settings` + `Printers[]`(持久化打印机注册表)。**打印机 ID 用 `NewPrinterID()`=时间戳+原子递增序号**〔**踩过的坑**:原来用裸 `time.Now().UnixNano()`,Windows 时钟精度粗(~100ns),数组一次登记多台时并发两次调用撞出**相同 ID**,状态表按 `p.ID` 存取导致两台状态互相绑定(关一台另一台跟着离线);FindPrinter/RemovePrinter 也会误取。**AddWizard/UpsertPrinter 一律走 `NewPrinterID`,勿再用裸 UnixNano**〕。`HealPrinterIDs()` 启动时自愈旧配置里空/重复 ID(`main` 加载后调用、有改动即落盘)。
- **日志 `internal/logger`**:全局文件日志(与 exe 同目录 `congmingpay.log`)。**业务代码一律调用 `logger.Info/Infof/Error/Errorf`,不直接用标准库 `log`**;`main` 启动时 `logger.Init(path)`。GUI 无控制台,靠此排查。
- **工具 `internal/util`**:`ToGB18030`(x/text 编码)。
- **资源 `internal/assets`**:`//go:embed app.png` 图标,保证单 exe。
- **UI `internal/ui`**(Walk,按视图/职责拆文件):
  - `App.go` 外壳(窗口 + 左导航 ListBox + 内容页切换 + 状态栏)。
  - **状态监测 `StatusMonitor.go`——每台打印机一条后台持续 ping(等价 `ping -t`,独立通道)**:`syncMonitors()` 让监测与打印机列表对齐(缺的起、连接身份 `monSig`〔conn/ip/port/usbName〕变了停旧起新、删的停);`monitorLoop(snap, stop)` 每 `pingInterval`(1s)一次 `printsvc.Status(&snap)`(网口 `transport.QueryICMPPing`〔ICMP,不占 9100〕/ USB winspool),**不做防抖——超时即离线、通即在线**,仅状态标签变化时 `mw.Synchronize` 写 `printerModel.status[id]`+`refreshPrinters()`+记日志 `打印机状态: [id=…] 名称『…』addr → 标签(detail)`。`snap` 为起监测时的值拷贝(不读共享指针,避免与属性编辑竞态)。`Run` 启动 `syncMonitors()`、退出 `stopAllMonitors()`;增删/属性/云端同步后 `syncMonitors()`。**踩过的坑**:早先用"单定时器同步批量探测所有机",与真离线机(占满 1s 超时)并发会串扰、导致在线机瞬时误报离线——改成每台独立 ping 后消除。`statusInfo` 带 `detail`;`main` 启动打印机清单(含 ID)落日志。
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

- **真机验收**(需硬件):USB=先装 Windows 驱动 → 搜索/刷新 → 选中 → 打印;网口=填 IP → 打印(走 9100)。在 Win7 SP1 上复测兼容性。
- **云端协议与 SDK**(MQTT?字段?)—— 用户后续单独说明,勿臆测。
- **Android 端**方案 —— 以后独立进行。
- 386 单架构是否满足全部目标机(如需再加 amd64:另出 `rsrc_windows_amd64.syso`)。
