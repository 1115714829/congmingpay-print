# CLAUDE.md（Android 端）

This file provides guidance when working with the Android portion of this repository.

## 项目定位

面向**餐厅厨房**的**热敏小票打印服务器**。

- 目标设备:**58mm / 80mm 热敏小票打印机**,主用 **ESC/POS** 指令。
- 最终形态:**云端下发指令 → 本地前台服务执行 → USB/蓝牙/网口打印**。
- 长期目标:**多品牌**、**USB + 蓝牙 + 网口**多通道。
- 本子项目是 **Android 端独立实现**,与同级 Windows 端（上级目录根目录)**不共享任何代码或文件**。两端协议一致、行为对齐,但各自独立开发、独立构建、独立维护。后续项目成熟后会物理拆分为独立仓库。

## 当前阶段:多打印机管理控制台（Android 版）

功能与 Windows 端对齐,交互结构还原设计稿:

- **左导航**(打印机 / 打印队列 / 系统设置)+ 内容区 Fragment 切换 + **底部状态栏**(`共 N 台 · N 在线 · N 任务进行中` + 临时消息)。
- **打印机视图**:工具栏(新增/刷新/测试打印/打印样票/**JSON测试**〔填 JSON 单对象或数组,本地直接走 `PrintProcessor`,等价 MQTT 下发〕/属性/删除)+ 筛选(全部/58/80/在线)+ 搜索 + 多列 `RecyclerView`(名称/来源〔local 本地添加·cloud 云端下发〕/品牌/规格/连接/地址/彩色状态/**上次打印时间**〔成功出纸绝对时间〕)。
- **打印队列视图**:任务表(预览/重新打印/取消/清除已完成),状态彩色;**任务落盘 `jobs.db`(SQLite,Room)**,重启恢复未完成并续打;JSON 任务另存 `source_json` 供「预览」出图。
- **系统设置**:服务名(**用作前台服务通知标题**,保存即时生效、空回退默认) + **系统通知开关**(`NotifyDisabled` 反向字段,默认开,保存即时生效) + **云盒兼容模式**(`YunheCompatDisabled` 反向字段,默认开=C1–C4,保存即时生效) + **开机启动**(`bootStartEnabled`,默认开) + **悬浮窗权限引导**(告警球) + **打印历史保留天数**(`JobHistoryDays`,默认7,仅清 done/failed) + **云端 MQTT**(Tab 三选一:`generic` 自建 · `aliyun` 云消息队列 · `iot` 物联网一机一密;`Resolve`统一产出;**启用时校验当前 Tab**) + **本地接口文档服务**(启用/端口/局域网地址,只读文档,NanoHTTPD) + 保存。
- **新增打印机向导**:佳博 SDK `PrinterFinder` 搜索(USB/蓝牙/网口/串口一次搜)→ 选择 → 配置名称/品牌/纸宽。
- **属性页**:蜂鸣/首尾空行/切刀/品牌/规格/连接信息。
- **多打印机 + 并发打印**:每任务独立协程;**离线转「等待重试」队列、联通即打**(唯一自动重试,仅连接类、不限次数);状态监测每台独立 1s 巡检。

> 云端通道 = **MQTT(唯一,`mqtt/` 包)**:Provider 三选一——**自建**(用户名密码,订阅短商户号,上报手填主题)、**云消息队列 MQTT 版**(AK/SK;`GroupId@@@自定义ID`;主题 `{父主题}/{自定义ID}/{上下行后缀}`)、**物联网平台**(一机一密 ProductKey/DeviceName/DeviceSecret;主题 `/{pk}/{dn}/user/{上下行后缀}`;设备列表预留)。payload=`PrintRequest` 单/数组收打印;上行 **report** / **state**(定时+查询+LWT);每条带 `merchant`(自建=短商户号,云消息队列=自定义ID,物联网=DeviceName)。

## 技术栈(已定)

- **语言:Kotlin。** 最低 **minSdk 21**(Android 5.0),**targetSdk 35**(Android 15)。后续新版本发布再跟进 targetSdk。
- **UI:Android View + XML 布局**(兼容性好,minSdk 21 友好)。
- **后台运行:前台服务(`Foreground Service`)** + 常驻通知栏 + 电量优化白名单申请。前台服务类型 `connectedDevice`。
- **打印 SDK:佳博 sdk2.aar**(USB/蓝牙/网口/串口统一连接,`Printer.connect` + `ConnectionListener` 回调)。
- **连接模型:短连接**——每次打印 `connect → print → disconnect`,不独占打印机(允许其他 app 使用同一打印机)。与 Windows 端一致。
- **协程:Kotlin coroutines**(`suspendCancellableCoroutine` 桥接 SDK 异步回调为 suspend 函数)。
- **JSON:Gson**(反射式,与 Go 端 `encoding/json` 行为最接近)。
- **SQLite:Room**(TypeConverters 处理 `ByteArray` / Printer JSON 快照)。
- **MQTT:org.eclipse.paho:org.eclipse.paho.client.mqttv3:1.2.5**(paho Java,与 Windows 端 paho Go 对等)。
- **强制支持 Android 5.0(API 21)→ 最新版本。**

## 代码规范(项目规则)

- **命名:英文大小驼峰。** 标识符(类/函数/方法/变量/常量)用驼峰:**`PascalCase`**(类/接口/对象)与 **`camelCase`**(函数/属性/变量)——遵循 Kotlin 惯例。文件名也用**大驼峰**(如 `MainActivity.kt`、`Printer.kt`、`EscposBuilder.kt`)。
- **模块化:按职责拆包、拆文件,不把代码堆进单文件。** 每个功能模块(escpos/layout/api/mqtt/printer/store/config/ui 等)是**独立包**,修改某模块只动该包——如改 MQTT 只动 `mqtt/` 包,不影响其他。`tree` 要能一眼看清各模块。
- **包名全小写**(如 `escpos`/`layout`/`mqtt`/`printer`/`store`/`config`/`ui`),这是 Kotlin/Java 规定,不与文件名驼峰规则冲突。
- **与 Windows 端零共享:不 import 根目录任何 Go 代码,不引用 Windows 端文件。** 两端各自完整,协议对齐靠行为一致(同输入同输出),不靠代码复用。
- **平台差异收敛在传输层/连接层之下**,上层调度/渲染/MQTT/配置/模型逻辑保持与 Windows 端等价。

## 文档规范(硬性)

适用于对外标准文档(`接口文档.md`、`功能说明.md`):

- **只陈述现行为。** 禁止口语与第二人称指令(「你/请去看/打完去」),禁止编辑性口头补充。
- **禁止历史相对措辞**:「早期版本」「旧版本」「升级时」「不再」「已移除」「曾」「已修」等一律不得出现——行为变更后直接改写为新行为的陈述句。
- 术语统一(打印回执(report)/状态快照(state)/错误码(code)/本服务/打印机/短商户号/品牌「其他」);正文全角标点,字段名/取值/JSON 用代码体。
- **不得替用户臆定协议参数**:云端固定参数(如上报主题)无默认值、不自动派生,由用户手动填写;拿不准的值先问用户。
- 例外:CLAUDE.md 本身为内部开发备忘,不受上述文风限制。
- 接口文档的权威源在上级目录 `../接口文档.md`(Windows 端维护),Android 端协议须与其一致。

## 架构(分层,每层独立包)

- **传输/连接层 `printer/ConnectionManager`**:封装佳博 SDK——`PrinterFinder` 搜索(USB/蓝牙/网口)、`Printer.connect(PrinterDevice)` 异步连接、`printer.print(byte[])` 同步打印、`printer.disconnect()` 断开、`ConnectionListener` 全局回调路由(按 `printer.getPrinterDevice()` 标识符匹配 pending 连接)。设备标识符:网口=`WifiPrinterDevice.getIp()`、蓝牙=`getBluetoothDevice().getAddress()`、USB=`getUsbDevice().getDeviceName()`。短连接模型:每次打印 connect→print→disconnect。
- **在线检测 `printer/StatusMonitor` + `printer/OnlineDebouncer`**:每台打印机一条独立协程 1s 巡检。网口走 `Runtime.exec("ping -c 1 -W 1")`(系统 ping 二进制,免 root,不碰 9100);USB 走 `UsbManager.getDeviceList()` 查设备在场;蓝牙走 `BluetoothAdapter.getBondedDevices()` 查配对状态。**网口 30 秒离线防抖**(连续失败持续满窗才判离线、任一次 ping 通立即回在线);USB/蓝牙即时生效。三边沿分离:①原始成功边沿→`NudgeOnline` 唤醒等待任务(联通即打);②生效 label 边沿→主线程刷新 UI+日志;③生效布尔边沿→系统通知+告警窗 raise/resolve。**在线/离线两态**(与 Windows 一致;不做缺纸/开盖)。网口 30s 离线防抖;生效态每轮写入 `onlineMap`,detail 公式与 Windows 一字不差(`就绪(ping Nms)` / `离线(…;已持续 Ns)` / 缺项 `检测中`);MQTT `state`/`report` 固定文案对齐 Windows(`已提交`、未知 query 后缀等)。ping 失败时回退 `InetAddress.isReachable()`。
- **指令层 `escpos/`**:`EscposBuilder`(链式)构建 ESC/POS——`1B 40` 初始化、`1B 61 n` 对齐、`1B 45 n` 加粗、`1D 21 n` 倍宽高、`1D 56 01` 半切、`0A` 换行;**中文经 GB18030 编码**(`String.toByteArray(Charset.forName("GB18030"))`)。`buildTestReceipt(now, widthMM)` 生成测试小票(按纸宽自适应)。纸宽自适应:`charsPerLine`—58mm=384点=32字符、80mm=576点=48字符。重印抬头 `ReprintBanner(widthMM)`。蜂鸣 `buildBuzzer`=`ESC B n t`(手册命令62,厂商扩展,多品牌接入时需适配)。二维码 `GS ( K` Model 2、条码 CODE128/CODE39 = `GS k` + `GS h/w/H`、图像光栅化 `GS v 0`(含 Floyd-Steinberg 抖动)。
- **JSON 排版渲染 `layout/`**:`LayoutRenderer.render(contents, widthMM, compat)` 把云端 MQTT type=0 的 JSON 小票排版数组渲染为 ESC/POS。**全部严格模式**:字段缺失=文档声明默认(合法);字段存在但非法 → **整单拒绝**,error 带 `contents[i](种类)` 定位,ack ok:false + 落日志,不产出部分字节。合法值:size=两位 0-7(text/表格/both_sides)、align=left/center/right、表格列宽 1-100 整数**总和恰 100**、行单元格数=列数、单元格仅标量、both_sides 恰 2 段、**qrcode size=两位 01-09(缺省 04)**、**条码 size=两位 AB(A 模块宽 1-6 夹取≥2 / B 高度档 0-9,高度=(B+1)×24 点)**·hri 0-3、**bc128/bc128a 仅大写字母+数字且 ≤14 位、bc128c 仅数字且 ≤28 位**、**qrcode 内容 ≤512 字节(单/双码每段)**、**png 解码后 ≤60K**、**bmp 须 BMP 魔数且 ≤6K**(BitmapFactory 解码)、plugin cont 0/1 开钱箱(`ESC p`,t1/t2=25)、未知元素 type 拒(`cut` 在 compat=false 时也按未知 type 整单拒——切纸只由顶层 `cut` 字段控制;compat=true 时接受:cont "0"=全切、"1"=半切均触发切纸,false/"off" 保留不切)、有 tbody 无 thead 拒。支持元素:`title`(居中二倍字**不加粗**)/`text`(**cont 数组=多行**)/`div_line`·`div_star`/表格(**line_space=ESC J 点数**,8点=1mm)/`both_sides`/`qrcode`(单码 native、双码 ZXing 位图合成)/`bc128`·`bc128a`·`bc128c`(码集 A/A/C)·`code39`/`png`/`bmp`/`plugin`/`cut`(仅 compat)。
- **消息处理核心 `api/PrintProcessor`**:`process(cfg, svc, req) -> ProcessResult{jobNo, Printer 快照}, changed, err`(错误带 `errcode` 全局错误码) + `parseRequests(raw) -> List<PrintRequest>, wasArray, err` + `resolveTarget` + 请求模型(`PrintRequest`/`PrinterRef`)。供 **MQTT** 与 UI「JSON测试」复用。
  - **打印消息(`PrintRequest`)**:目标二选一 `printer`/`gateway`;type〔**0** 或云盒兼容下 **5**=JSON 排版 / **1**=ESC base64〕;pWidth、pCopy、contents;**云盒兼容模式**(`Settings.yunheCompat`,默认开)控制 C1–C4:开=`type:5`、五参数可省、contents `cut`、MQTT 仅 `done`/`failed`;关=严格协议+完整 `accepted`/`waiting`。
  - **随打印自动登记打印机(建列表)**:`resolveTarget` 目标**带 IP** 且**未注册** → `ConfigManager.upsertPrinter` 自动新增;**登记必须能确定纸宽**:`printer.width`→`pWidth` 回退链,两者皆缺/非法即拒。USB/蓝牙与"仅名/ID"目标不自动登记。
  - **pCopy 展开**:在 `process()` 中按 copies 次调用 `submit()`,每次生成独立任务号;串行锁保证同台打印机多份串行打印。
- **打印调度 `printer/PrintDispatcher`**:每任务开协程 → **并发打印**;`retry`/`cancel`/`clearDone`;**任务持久化 `store/JobStore`(Room SQLite)**:Submit/状态变更 UPSERT、Cancel/ClearDone DELETE、`restoreAndResume` 启动灌入(`JobPrinting`→`JobQueued` 再 dispatch)、`pruneHistory`/`setHistoryDays` 仅删超期 done/failed。`submit(p, doc, data, options)` 的 `Options`(cut/buzzer/head/tail/**reprint**/**cloudID**)按需覆盖每台默认。**任务事件**:`setOnJobEvent` 回调,`JobEvent{event(done/failed/waiting), code, jobNo, cloudId, printer, err}`;done/failed 终态各恰一次;waiting 非终态(持续被拒超 20s 触发,code=5101)。**`eventDone` 时写打印机 `LastPrint` 并落盘**。**在线注册表**:`setPrinterOnline(id, online, detail)` / `onlineSnapshot()`——StatusMonitor 每秒巡检写入保持新鲜,state 快照合成时读。
  - **dispatch 状态机(短连接 + 协程)**:① 首次派发先 `statusCheck()` 判在线(仅网口 ping;`entry.gated` 只门控一次)——离线则直接进 `JobWaiting` 省空连接;② 真打(`connectionManager.connect(device)` → `printer.print(data)` → `printer.disconnect()`,串行锁内)为最终裁决:成功→`JobDone`;连接失败/超时→`JobWaiting` 退避重试;打印回调失败→`JobFailed`(避免重复出纸)。③ 退避倍增:占用短退避(1→2→3s)/离线退避(5→10→20→30s);`select` 等待退避定时器或 `wakeChannel` 唤醒。④ NudgeOnline 唤醒等待任务、重置退避立即重打。⑤ gen 代次守卫:Retry 自增,旧协程在检查点退出(防重复打印)。⑥ reprintNext 标志:Submit 时 opts.reprint 或 Retry 时置位,dispatch 前置 ReprintBanner。
  - **收尾组装**:蜂鸣 → 重印抬头 → 首部空行(HeadLines) → 内容 → 尾部走纸(TailLines)+ 切刀。内容构建(`buildTestReceipt`、`LayoutRenderer`)不自带走纸/切纸。
- **配置 `config/ConfigManager`**:app 私有目录 `getFilesDir()/config.json`(Gson 序列化)—— `Settings` + `Printers` 列表(持久化打印机注册表)。**打印机 ID 用 `newPrinterId()`=高精度时间戳+`AtomicLong`递增序号**。config.json 损坏处理:备份为 `config.json.bad-<时间戳>` 再用默认配置启动。
- **数据模型 `model/`**:`Printer`(名称/品牌/规格/连接〔network/usb/**bluetooth**〕/IP·端口/USB设备名/**蓝牙MAC**/来源/上次打印/蜂鸣/首尾空行/切刀)、`Job`(任务+状态,状态含 `JobWaiting`「等待重试」)、`Settings`(服务名/通知开关/云盒兼容/**开机启动 `bootStartEnabled`(默认开)**/历史天数/MQTT/DocServer)。
- **云端通道 `mqtt/`(唯一)**:`MqttClient`(paho Java,MQTT 3.1.1)持 cfg/svc/cfgPath/onChange/onStatus。`start/reload(settings.mqtt)/close`;明文 + 用户名密码;**订阅 `<短商户号>`** 收打印 → `parseRequests`→`process`→回执;**发布到配置的「上报主题」**(`ReportTopic`)。两类上行且每条带 `merchant`:**打印回执 `report`**(accepted/waiting/done/failed) + **状态快照 `state`**(定时 1 分钟 + 查询 `{"query":"printers"}` + LWT 离线 + Close/Reload 收尾主动发)。报文构造集中在 `mqtt/Report.kt`,与 Windows `internal/mqtt/Report.go` 逐字段对齐:**accepted 带 `printer` 身份段 + `params` 本单生效参数**(buzzer/cut/reprint 为 int 0/1);done/failed/waiting 的 `printer` 仅身份字段(无 online);**`omitempty` 语义**(空 `ip`/`port`/`usbName`/`detail`/`lastPrint`、`jobNo==0`、空 `printers` 整键不出现);`id` 用 Long 回显防 uint32 大值溢出。三 Provider 签名(`Resolve`/`Iot`):generic 用户名密码 / aliyun HMAC-SHA1 / iot 一机一密。ClientID=短商户号(不加随机后缀)。自订阅回环守卫(上报主题==订阅主题→拒)。publish 代际校验(传入 cli≠当前 cli 即跳过)。best-effort 但有据可查。受理失败回调 `onAcceptFailed` → 悬浮告警窗。
- **前台服务 `service/PrintService`**:常驻运行 MQTT + 打印队列 + 状态监测 + **悬浮告警球**。通知栏常驻(前台服务通知,标题=服务名)。电量优化白名单申请。启动时恢复未完成任务。`START_STICKY`。
- **开机自启 `service/BootReceiver`**:`BOOT_COMPLETED` 时若 `bootStartEnabled` 则 `startForegroundService(PrintService)`。通用设置页开关控制。
- **单实例**:`MainActivity` 的 `launchMode=singleTask`;前台服务已运行判断。
- **日志 `logger/`**:全局文件日志(`getFilesDir()/log/<yyyy-MM-dd>.log`,按天轮转,保留 7 天)。**业务代码一律调用 `Logger.info/warn/error`**。
- **悬浮告警球 `ui/AlertOverlay` + `ui/AlertModel`**:平时屏幕边缘可拖拽小圆点;故障 `raise` 后**自动展开**面板(发生时间/已持续/内容,文案与 Windows 告警窗对齐);手动点圆点也可展开;按钮「打开 APP」「全部知道了」「收起」。种类 job-failed/job-waiting/printer-offline/mqtt-down/accept-failed;kind+key 幂等;上限 200;「已持续」中文 `N秒`/`N分SS秒`/`N时M分`;**不受** `notifyDisabled` 控制。需 `SYSTEM_ALERT_WINDOW`。保活分层:①前台服务主保活 ②悬浮窗可见驻留(部分 ROM) ③开机自启——**不神话单靠悬浮窗防杀**。
- **系统通知**:`NotificationManager` 通知渠道——打印失败/卡单 waiting/打印机离线·恢复/MQTT 断线·恢复。受 `Settings.notifyDisabled` 控制。
- **预览 `preview/`(低优先级)**:Android Canvas + 系统字体(NotoSansCJK)渲染小票预览。
- **接口文档服务(低优先级)**:NanoHTTPD 只读页,局域网内可访问。

品牌差异全部收敛在连接层与指令层之下。

## 佳博 SDK —— sdk2.aar

### 文件位置（全部相对路径）

| 用途 | 路径（相对项目根目录） | 说明 |
|------|----------------------|------|
| **构建依赖** | `Android/app/libs/sdk2.aar` | **唯一构建来源**，自包含，不引用上级 SDK 目录 |
| 原始 tar | `SDK/Android新框架-SDK开发包.tar` | 原始压缩包，保留不动 |
| API 手册 | `SDK/_extract_android/<开发包>/安卓SDK2-2.0.3-编程手册.pdf` | 佳博 SDK API 手册（核心参考） |
| 适配文档 | `SDK/_extract_android/<开发包>/SDK适配16KB.pdf` | 16KB 页对齐适配 |
| demo 源码 | `SDK/_extract_android/demo/<示例demo>/printer-sdk-demo-dev/.../src/main/java/com/gainscha/sdk/demo/` | 示例代码（API 用法参考） |

> **注**：`<开发包>` 和 `<示例demo>` 是中文文件夹名（佳博SDK2-2.0.3-开发包 / 示例demo），因 tar 内 GBK 编码在部分终端显示为乱码，文件本身完好。查找文件时用递归通配（如 `Get-ChildItem -Recurse -Filter "*.pdf"`），不硬编码中文路径。从 `Android/` 目录出发，上级 SDK 目录为 `../SDK/`。

### 构建配置

- `app/build.gradle` 用 `flatDir { dirs 'libs' }` + `implementation(name: 'sdk2', ext: 'aar')` 引入 AAR。
- 传递依赖（AAR 不自带，需手动声明）：`com.gainscha:serial-port`、`com.gainscha:jzint`、`com.gainscha:fimage`、`com.jcraft:jzlib`。

### 连接模型

- **短连接**——每次打印 `Printer.connect(PrinterDevice)` → `onPrinterConnected` 回调 → `printer.print(byte[])` → `printer.disconnect()`。不维持长连接，不独占打印机。

### SDK API 速查

- 见 `../.kilo/plans/1785402161849-android-migration-plan.md`「七-B」节（反编译 classes.jar 确认的方法签名）。
- 完整 API 以 `../SDK/_extract_android/` 下的 PDF 手册为权威。

## 构建 / 运行

- **构建工具:Gradle + Android Gradle Plugin**。`minSdk 21`、`targetSdk 35`、Kotlin 1.9+、Java 8 sourceCompatibility。
- **构建方式(硬性):一律用 Android Studio IDE 构建**,经 MCP `android-studio-index` 的 `ide_build_project`(全量重建 `rebuild=true`)/`ide_diagnostics`(错误定位)完成——**禁止直接跑 gradle 命令行**。本机无 `gradlew` wrapper;仓库内 Gradle 9.0.0 发行版与 AGP 8.1.4 不兼容(报 `DependencyHandler.module(Object)` 错误),命令行构建必然失败,IDE 构建是唯一可信通道。
- **MCP 构建通道行为(踩坑实录)**:`ide_build_project` 是**异步提交**,返回 `durationMs` 仅 1-6ms 属正常,**不是**空操作;真实结果看 `ide_diagnostics(includeBuildErrors=true)` 的 `buildTimestamp`(每次构建后刷新)与 `buildErrors`(空=成功)。但该通道依赖 IDE 的 Gradle 模型——脚本/依赖报错导致 Sync 失败后模型停在失败态,构建请求会被静默跳过,此时须先请用户在 Android Studio 里 `File → Sync Project with Gradle Files` 恢复模型再构建。
- **环境事实(本机)**:
  - Android Studio:`D:\Program Files\Android\Android Studio`(进程 studio64.exe,自带 JBR 21)
  - Android SDK:`D:\Androidsdk`(记录于 `local.properties` 的 `sdk.dir`)
  - Gradle 用户目录:`GRADLE_USER_HOME=D:\gradle-repo`(含 wrapper dists 9.0.0/8.11.1)
  - **Gradle 版本必须 8.11.1、Gradle JVM 必须用 AS 自带 JBR 21**(Gradle 8.11.1 支持 Java 8-23;系统 JDK 24 不兼容,AS 报 "Incompatible Gradle JVM version")
  - 无 `gradlew` wrapper、无系统 gradle——不要尝试 `gradle`/`./gradlew` 命令
- **构建脚本语法(踩坑实录)**:`*.gradle` 是 **Groovy DSL**——`buildTypes.release` 里必须写 `minifyEnabled = false`,`isMinifyEnabled` 是 Kotlin DSL 专属写法,Groovy 下报 `Could not set unknown property`。`settings.gradle` 声明了 `RepositoriesMode.FAIL_ON_PROJECT_REPOS`(禁止项目级仓库),任何 `repositories{}`(含 AGP `android.repositories` 的 flatDir)必须集中声明在 `settings.gradle` 的 `dependencyResolutionManagement.repositories` 里,flatDir 的 `dirs` 相对 settings 目录 = `Android/` 根,故写 `dirs("app/libs")`;AAR 依赖用 `implementation(files("libs/sdk2.aar"))` 文件依赖,不走仓库解析。
- **打包:APK**。调试 `assembleDebug`,发布 `assembleRelease`(均由 Android Studio 执行)。
- **佳博 SDK 16KB 页对齐**:targetSdk 35 + Android 15 可能要求 16KB 对齐,按 SDK 适配文档配置 so 库。
- 日志看 `getFilesDir()/log/<日期>.log`。

## 工作规则(硬性)

- **禁止臆想,必须有理有据。** 不确定的库/接口/参数不得编造;以佳博 SDK demo、手册、或代码中可验证内容为依据;无依据时标「待确定」并向用户确认。
- 标为「待确定」的项,用户拍板前不要自行补全为具体方案。
- **两端独立,不共享文件**:修改 Android 端只动 `Android/` 目录下文件,不碰上级目录 Windows 端代码。所有路径用**相对路径**(从 `Android/` 目录出发,上级为 `../`),不硬编码盘符(如 `F:\`),确保换 Mac/Windows 电脑或迁移目录后路径仍有效。
- **协议对齐**:云端 MQTT 消息格式、错误码、JSON 排版严格模式拒绝点须与 Windows 端行为一致(同输入同输出),对齐靠单元测试 fixture 共享(复制 Windows 端测试用例到 Android 测试资源),不靠代码复用。**打印模板渲染(Android `layout/` 与 Windows `internal/layout/`)两端必须同步修改并各加回归测试**——尤其放大(`size`)场景:渲染层不报超宽,换行发生在打印机硬件,回归须断言单元格/行物理宽≤纸宽(见根目录 AGENTS.md 工作规则)。
- **云盒兼容模式**(C1–C4)见上级目录 `../旧版兼容.md`,设置开关默认开。

## 待确定 / 下一步

- **真机验收**(需硬件):USB/蓝牙/网口三种连接打印 + MQTT 上下行 + 离线排队联通即打 + 告警窗。
- **SDK 连接失败错误分类细化**:当前简化为单档退避;后续可用 ping 辅助分类(ping 通=占用短退避,ping 不通=离线长退避)。
- **Runtime.exec ping 兼容性**:部分 ROM 可能限制 ping 二进制,需真机验证覆盖主流 ROM;已准备 `InetAddress.isReachable()` 回退。
