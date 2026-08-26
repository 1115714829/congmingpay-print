# web 管理系统设计(打印服务器 SN/DeviceName 管理中心)

> 修订版 v4(本版新增:库存 + 分配逻辑 + CSV 入库模块):
> - 核心功能:「设备硬件指纹 → 商户号认领 DeviceName → 唯一绑定,重装系统不变」。
> - **入库**:设备证书 CSV(三列 `DeviceName,DeviceSecret,ProductKey`,模板即 `E:\congmingpay-print\设备证书.csv`)导入**库存**;格式不对、行内排重或与库存已有 DeviceName 重复 → **整批禁止入库**(不部分入库)。
> - **分配**:商户管理页行内「分配设备」按钮 → 输入数量 → 直接从库存提取未分配设备给该商户,**商户独占,保留 1 个月**;1 个月内未激活(未绑定)→ 自动回库存。
> - 商户号本期**不与其他任何系统关联**;商户管理页**只做新增**(新增完成即入库);商户有**长/短商户号**,设备端输入任一个都可识别。
> - 设备管理页只读展示(表头:设备SN/设备密钥/通信密钥/所属商户/分配时间)。
> - 打印机上报/MQTT/点餐系统通信相关代码(internal/mqtt、internal/printsvc、internal/api 等)**一律不动**。

## 0. 第一步(用户明确要求):先提交 commit

当前未提交改动(上轮 MQTT 预置参数/参数隐藏/超管配置/崩溃修复)先提交再开工:
- `git add internal/model/Settings.go internal/config/Config.go internal/ui/App.go internal/ui/SettingsView.go`
- commit 消息(跟随仓库风格):`MQTT默认切换物联网平台并预置端点与上下行后缀;设置页参数默认隐藏+超管配置会话解锁;修复订阅上报预览0宽不显示与超管对话框nil崩溃`
- 不 push(用户未要求)。

## 1. 核心业务流程(本次设计的锚点)

### 1.1 设备端认领(未来 Windows 主程序/安卓,本期只定 API 不改主程序)
1. 用户点「获取设备信息」→ 客户端读取本机**硬件指纹**(主板序列号/CPU ID/磁盘序列号/MAC,重装系统不变)→ 提示输入**商户号**(长/短均可)。
2. `POST /api/v1/device/lookup {merchantNo, fingerprint}` → 后端按商户号(长或短)查商户:
   - 商户不存在 → `2003`;
   - 指纹已绑定过某 DeviceName(重装后场景)→ 返回 `boundDeviceName`(客户端提示「已绑定 X,是否恢复」);
   - 该商户名下无未绑定设备(未分配/已过期回收)→ `3001`(请商户在管理端先分配设备);
   - 否则返回 `availableDeviceNames`(该商户名下已分配且未绑定的,1 个月独占期内)。
3. 用户选择(或确认恢复)→ `POST /api/v1/device/bind {merchantNo, deviceName, fingerprint, deviceInfo}`:
   - DeviceName 属该商户 且 未绑定 → 原子置为 bound(抢注保护:并发只有一个成功);
   - 指纹已绑定其他 DeviceName 且选择不同 → 拒绝(返回已绑定的,提示恢复原绑定);
   - 幂等:同一 SN 重复 bind 同一 DeviceName 成功并刷新 lastSeen(兼作心跳)。
4. 客户端把选定的 DeviceName 写入本地 MQTT 配置 → 连接物联网平台(原有链路,不动)。

**唯一关系**:1 DeviceName ↔ 1 硬件指纹(SN)。管理员可强制解绑(换硬件/回收),普通流程不开放。
**重装系统不变**:绑定存云端,身份是硬件指纹;重装后同硬件 → 同指纹 → lookup 直接返回原绑定,一键恢复。

### 1.2 库存与分配(管理端,本期实现)
- **库存**=已入库且未分配给任何商户的设备。
- **分配**:商户页「分配设备」→ 输入数量 N → 后端从库存按入库顺序取 N 个未分配设备,置为"已分配给该商户"(记录分配时间),**商户独占**;库存不足 N 时报 `3006 库存不足`。
- **1 个月独占期**:分配后 30 天内该商户可认领;到期仍未绑定 → 设备回库存(清商户归属与分配时间),下次分配可再取出。到期扫描:后端启动时 + 每 1 小时定时跑一次 `expireAllocations()`(SQL: `WHERE allocated_at < now-30d AND bound_fp_hash IS NULL`)。
- **回收**:管理端可对已分配未绑定的设备「收回」(立即回库存)。

## 2. 已确认决策
- 独立云端服务(web 目录自包含,后端可部署 Linux);本期只做 web 端,主程序不动。
- Go 后端 + Vue 前端;API 文档是前后端通信唯一标准;`web/CLAUDE.md` 写规则。
- 商户号不与其他系统关联;商户管理页**只新增**;长/短商户号双字段,输入任一可命中。
- 设备入库 = CSV 三列导入,带排重、格式强校验,失败整批拒绝。
- 分配 = 从库存提取 N 个,商户独占 1 个月,未激活自动回库存。
- 打印机上报相关(internal/mqtt、internal/printsvc、internal/api)**零改动**。

## 3. 目录结构

```
web/
├── CLAUDE.md               # 规则文件:API 文档=唯一通信契约、变更顺序、命名/错误码、指纹 canonical 规则、入库模板与校验规则、分配/独占期规则、安全清单
├── api/
│   ├── API.md              # 完整接口文档(唯一参考物,人读;每接口含请求/响应字段表+示例+错误码)
│   └── openapi.yaml        # OpenAPI 3.0 同份契约(机器可读)
├── backend/                # 独立 Go module:congmingpay/web (go 1.20)
│   ├── go.mod
│   ├── main.go             # env 配置 → 打开 DB → 迁移 → 种子管理员 → 启动到期回收定时器 → 路由 → ListenAndServe
│   └── internal/
│       ├── config/         # 环境变量读取
│       ├── store/          # SQLite:建表迁移 + 仓储(merchants/devices/fingerprints/accounts/logs)
│       ├── auth/           # bcrypt + JWT(HS256) + Bearer 中间件
│       ├── fingerprint/    # 指纹 canonical 序列化 + SHA256 摘要 + 合法性校验
│       ├── inventory/      # CSV 导入解析与校验 + 分配/回收/到期逻辑
│       └── api/            # 路由分发 + handler:Login/Merchant/Device/Inventory/DeviceReport/Account/Dashboard
└── frontend/               # Vue 3 + Vite + vue-router + pinia + element-plus + axios
    └── src/
        ├── api/http.js     # axios 实例(Bearer 注入、code!=0 统一报错)
        ├── stores/auth.js
        ├── router/index.js # 登录守卫 + admin/operator 菜单差异
        └── views/{Login,Dashboard,Merchants,Devices,Accounts,Profile}.vue
```

依赖:后端 `golang-jwt/jwt/v5`、`golang.org/x/crypto`(bcrypt)、`modernc.org/sqlite`、标准库路由与 `encoding/csv`;前端 `vue@3/vue-router@4/pinia/element-plus/axios`,Vite 5。

## 4. 数据模型(SQLite,表结构)

```sql
accounts(id, username UNIQUE, password_hash, display_name,
         role CHECK IN ('admin','operator'), enabled DEFAULT 1, created_at, updated_at)

merchants(id, merchant_no_long UNIQUE,   -- 长商户号
          merchant_no_short UNIQUE,      -- 短商户号(两者都唯一;允许相同值)
          name,                          -- 商户名称(分配/展示用)
          contact_phone, address, remark,
          created_by, created_at)
-- 设备端 merchantNo 匹配:先 long 精确匹配,未中再 short 精确匹配

devices(id, device_name UNIQUE,          -- 设备SN/DeviceName
        device_secret,                   -- 设备密钥(CSV 入库)
        product_key,                     -- 通信密钥(CSV 入库)
        merchant_id NULL FK→merchants,   -- 所属商户(NULL=库存;非空=已分配/已绑定)
        allocated_at NULL,               -- 分配时间(独占 1 个月起点;NULL=在库存)
        bound_fp_hash NULL,              -- 绑定指纹摘要(NULL=未激活)
        bound_at NULL,                   -- 绑定时间
        last_seen_at NULL,               -- 最近 lookup/bind 心跳
        os_type NULL,                    -- 客户端平台 'win'/'android'(设备端上报,设备页展示)
        app_version NULL,                -- 客户端版本号(设备端上报,设备页展示)
        created_at)                       -- 入库时间
-- os_type/app_version 由设备端 lookup/bind 携带的 deviceInfo 刷新(每次上报都更新),
-- 设备页列「平台/版本」展示(如 Windows 1.2.0 / Android 1.2.0);未上报过显示 "—"

fingerprint_bindings(fp_hash PK, device_name, raw JSON, first_seen_at, last_seen_at)
-- 指纹→DeviceName 反向索引,重装恢复靠它直接查回

login_logs(id, username, ip, ua, ok INT, reason, created_at)
```

状态推导(不单设 status 字段,按列推导,避免状态机不一致):
- **库存**: `merchant_id IS NULL`
- **已分配未激活(独占期内)**: `merchant_id IS NOT NULL AND bound_fp_hash IS NULL AND allocated_at 未过期`
- **已过期待回收**: 上一行且 `allocated_at < now-30d`(定时器把它置回库存)
- **已绑定(已激活)**: `bound_fp_hash IS NOT NULL`

## 5. CSV 入库规则(强校验,整批原子)

- 文件:UTF-8(允许 BOM)/CSV,首行表头必须恰为 `DeviceName,DeviceSecret,ProductKey`(忽略列首尾空格,大小写敏感),三列齐全,否则整批拒绝(`4002 模板格式错误`)。
- 逐行校验:三列均非空;DeviceName 仅字母数字(与现有 ZYDY000000000026 风格一致),长度 1-64;DeviceSecret/ProductKey 非空且长度 1-128;行内 DeviceName 重复(`4003 文件内重复`)、与库存/已分配设备重复(`4004 与库存重复`)均整批拒绝。错误响应带首个出错行号与原因。
- 成功:整批事务入库(库存态 merchant_id=NULL);响应返回 `{imported, total}`。
- 实现:`inventory.ImportCSV(r io.Reader)`,先全量校验再单事务 INSERT,任何一步失败 ROLLBACK,已存在零写入。
- API:`POST /inventory/import`(multipart 文件字段 `file`)。

## 6. 设备指纹规则(写死在 CLAUDE.md 与 API.md)

- **参与摘要(身份)的字段**——必须跨重装不变,全部硬件级:
  `osType`("win"/"android")、`boardSerial`(主板序列号)、`cpuId`(CPU ID)、`macAddress`(主网卡 MAC,大写)、`diskSerials`(磁盘序列号数组,内部排序)
- **不参与摘要(仅展示/记录)**:`osBuild`、`appVersion`、`deviceModel`
- `fp_hash = SHA256(canonicalJSON)`:字段按名排序、TrimSpace、diskSerials 排序后序列化。
- 合法性:boardSerial/macAddress/diskSerials 三者至少一项非空,否则 `3004 指纹无效`。
- 设备端采集来源(PC,供后续主程序实现参考):WMI `Win32_BaseBoard.SerialNumber`、`Win32_Processor.ProcessorId`、`Win32_DiskDrive.SerialNumber`、`Win32_NetworkAdapterConfiguration.MacAddress`;安卓:Build.BOARD/serial 等。本期只写进文档。

## 7. API 契约(完整字段/示例落在 API.md 与 openapi.yaml)

通用:`/api/v1`、JSON UTF-8、时间 RFC3339;信封 `{"code":0,"message":"ok","data":...}`,`code=0` 成功;业务错误 HTTP 200 + 业务码,仅 401/404/405 用状态码;管理端 `Authorization: Bearer <JWT>`(HS256,24h);设备端 `X-Device-Name` 请求头(可空,仅作日志)。

错误码:`1001 未登录/令牌过期` `1002 无权限(需admin)` `1003 用户名或密码错误` `1004 账号已停用` `2001 参数错误` `2002 用户名已存在` `2003 商户号不存在` `2004 商户号(长/短)已存在` `3001 该商户暂无可认领设备` `3002 DeviceName不存在或不属于该商户` `3003 该设备(SN)已绑定其他DeviceName` `3004 指纹无效` `3005 设备已被占用(抢注失败)` `3006 库存不足` `4001 文件读取失败` `4002 模板格式错误` `4003 文件内DeviceName重复` `4004 与库存DeviceName重复`。

**A. 账号**(Bearer):
| 方法 路径 | 说明 |
|---|---|
| POST /login | `{username,password}` → `{token,user}`;写 login_logs |
| GET /me | 当前账号 |
| PUT /me/password | `{oldPassword,newPassword}` |
| GET /accounts (admin) | `{page,pageSize,keyword}` 分页 |
| POST /accounts (admin) | `{username,password,displayName,role}` |
| PUT /accounts/{id} (admin) | displayName/role/enabled |
| PUT /accounts/{id}/reset-password (admin) | `{newPassword}` |

**B. 商户**(Bearer):
| 方法 路径 | 说明 |
|---|---|
| GET /merchants | 列表(含 已分配数/已绑定数 统计) |
| POST /merchants | `{merchantNoLong,merchantNoShort,name,contactPhone,address,remark}` → 新增入库;重复号 `2004` |
| GET /merchants/{id} | 详情(含名下设备汇总) |
| POST /merchants/{id}/allocate | `{count}` → 从库存取 count 个未分配设备给该商户(独占 30 天);库存不足 `3006`;返回分配到的 DeviceName 列表 |
| GET /merchants/{id}/devices | 名下设备(名称/状态:未激活含剩余天数/已绑定/最近上线) |
| POST /merchants/{id}/devices/{name}/reclaim | 收回未绑定设备回库存(已绑定拒绝 `2001`) |

**C. 设备(只读)**(Bearer):
| 方法 路径 | 说明 |
|---|---|
| GET /devices | 总表,列:设备SN(DeviceName)/设备密钥(DeviceSecret)/通信密钥(ProductKey)/所属商户/分配时间/**平台/版本(osType+appVersion)** + 状态(库存/已分配未激活/已绑定)+ 最近上线;`?keyword=&merchantId=&state=` 过滤;分页 |
| GET /devices/{name} | 详情(绑定指纹 raw、分配/绑定时间、平台/版本、最近上线) |
| POST /devices/{name}/unbind (admin) | 强制解绑已绑定设备(换硬件场景,清指纹绑定,设备回库存) |

**D. 入库**(Bearer,admin):
| 方法 路径 | 说明 |
|---|---|
| POST /inventory/import | multipart `file`(csv)→ 全量校验+整批入库;失败整批回滚并返回首个错误行;`{imported,total}` |
| GET /inventory | 库存列表(未分配设备)+ 库存总数(供分配前查看) |

**E. 设备端**(X-Device-Name;接口+文档本期交付,主程序/安卓后续实现):
| 方法 路径 | 说明 |
|---|---|
| POST /device/lookup | `{merchantNo(长或短), fingerprint, deviceInfo{osType,appVersion,osBuild,deviceModel?}}` → `{merchant:{...}, boundDeviceName?(指纹已绑则返回), availableDeviceNames:[...], total, boundCount}`;指纹未见过合法(首绑);刷新 lastSeen,若已绑设备则刷新其 os_type/app_version |
| POST /device/bind | `{merchantNo, deviceName, fingerprint, deviceInfo{osType,appVersion,osBuild,deviceModel?}}` → `{deviceName, deviceSecret, productKey, status:"bound"}`;返回密钥供客户端直接写入本地 MQTT 配置;同时写入该设备 os_type/app_version(设备页「平台/版本」列);规则见 1.1;幂等兼心跳 |

> `deviceInfo` 说明:`osType` 必填("win"/"android");`appVersion` 客户端版本号(如 1.2.0);`osBuild`/`deviceModel` 可选仅记录展示;均不参与指纹摘要(见第 6 节)。

**F. 总览**(Bearer):
| GET /dashboard | 商户数/库存数/已分配未激活数/已绑定数/近 7 天登录成功失败数 |

## 8. 后端实现要点

- **独立 module** `congmingpay/web`,go 1.20,零 CGO(modernc sqlite),Linux 可部署。
- **启动/env**:`WEB_PORT`(默认 9000)、`WEB_DATA`(默认 ./data)、`JWT_SECRET`(缺省内置值+强提醒)、`ADMIN_USERNAME`(默认 admin)、`ADMIN_PASSWORD`(默认 admin123);accounts 为空时种子创建。
- **启动流程**:open sqlite(WAL、busy_timeout=5000、SetMaxOpenConns(1))→ 迁移 → 种子 → 到期回收定时器(goroutine:启动即跑一次 + 每 1h,`UPDATE devices SET merchant_id=NULL, allocated_at=NULL WHERE merchant_id IS NOT NULL AND bound_fp_hash IS NULL AND allocated_at < now-30d`)→ 路由 → `http.Server{ReadHeaderTimeout:5s}`;SIGINT/SIGTERM → 500ms Shutdown(与主程序 docserver 同款)。
- **路由**:标准库 `ServeMux` + 自写 ~30 行分发器(`METHOD path` 精确表,`{param}` 单段参数),不引 Gin/Echo。
- **鉴权**:管理端 Bearer JWT(`{uid,username,role,exp}`,HS256,24h)注入 user;设备端仅校验指纹合法。
- **CORS**:开发期允许 `http://localhost:5173`。
- **分配事务**:`BEGIN → 取库存 N 条(ORDER BY created_at LIMIT N,行锁靠单连接)→ 更新 merchant_id/allocated_at → COMMIT`;N 取不到就 ROLLBACK 报 `3006`。
- **bind 事务**:`BEGIN → UPDATE devices SET bound_fp_hash,bound_at,merchant_id 校验归属(条件 bound_fp_hash IS NULL)→ 查/写 fingerprint_bindings → COMMIT`;抢注失败 3005;指纹已绑他设备 3003;设备已过期回库存(merchant_id=NULL)时 bind 拒绝 `3002`。
- **CSV 导入**:`inventory.ImportCSV`:encoding/csv 解析(首行表头精确匹配)→ 全量校验(三列非空/DeviceName 字母数字/文件内去重/与库去重,后者一次 `SELECT device_name FROM devices WHERE device_name IN (...)`)→ 单事务批量 INSERT;任何失败整批回滚。
- **商户号匹配**:`store.FindMerchantByNo(no)`(long 未中再 short)。
- **登录日志**:成败均写 login_logs(IP/UA/原因)。
- 日志沿用主程序风格:文件日志(data/log/日期.log)+ 启动横幅。

## 9. 前端页面与交互

- **登录页**:用户名/密码;token 存 Pinia + sessionStorage;401 全局拦截回登录页。
- **总览**:4 张统计卡(商户/库存/已分配未激活/已绑定)+ 近 7 天登录统计表格。
- **商户页**:表格(长商户号/短商户号/名称/联系人/已分配数/已绑定数)+「新增商户」按钮(对话框:长号/短号/名称/联系人/地址/备注,提交即入库;重复号提示);行内「**分配设备**」按钮 → 对话框显示当前库存数 + 输入数量(1≤N≤库存)→ 确认 → 展示分配到的 DeviceName 列表;行内「设备」按钮 → 抽屉看名下设备(未激活显示剩余天数、可「收回」)。本期不做商户编辑/删除。
- **设备页**:只读总表,表头:**设备SN(DeviceName)|设备密钥(DeviceSecret)|通信密钥(ProductKey)|所属商户|分配时间|平台/版本**,附状态/最近上线;「平台/版本」列展示 osType 图标(win/android)+ appVersion(如 "Windows 1.2.0"/"Android 1.1.3"),未上报过显示 "—";过滤(关键词/商户/状态)+ 分页;行详情展示绑定指纹 raw、osBuild/deviceModel;admin 对已绑定设备可「解绑」;顶部「**导入设备**」按钮(admin)→ 对话框上传 CSV(校验提示与后端一致的前端预检:表头/列数/文件内重复),成功后刷新。
- **账号页**(admin):列表 + 新建/编辑/重置密码/启停用。
- **个人中心**:改密码。
- `http.js`:baseURL `/api/v1`、自动 Bearer、`code!=0` 统一 ElMessage 报错;上传接口带 `multipart/form-data`。

## 10. 假设与决策

| 决策 | 取值 | 依据 |
|---|---|---|
| 核心链路 | 商户号 + 硬件指纹 → 认领/恢复 DeviceName,唯一绑定 | 用户明确 |
| 重装恢复 | 无恢复码/无人工审核,纯硬件指纹直接认领 | 用户纠正 |
| 商户 | 不与其他系统关联;长/短号双字段;管理页只新增 | 用户补充 |
| 入库 | CSV 三列模板(=现有 设备证书.csv);排重+格式强校验,失败整批拒绝 | 用户明确 |
| 分配 | 商户页按钮输数量,从库存提取,商户独占 30 天,未激活自动回库存 | 用户明确 |
| 到期扫描 | 启动即跑 + 每 1h 定时,纯 SQL 条件更新 | 实现最简且幂等 |
| 打印机上报/MQTT | 零改动 | 用户明确 |
| 部署/工程 | 独立云端服务;web 独立 Go module;标准库路由;JWT+bcrypt;modernc sqlite | 用户确认/轻量 |
| bind 返回密钥 | 成功返回 deviceSecret/productKey,客户端直接写入 MQTT 配置 | 选定保存后即可连接 |
| 默认管理员 | admin/admin123 + 启动强提醒 | 交付便利 |
| 文档 | CLAUDE.md + API.md + openapi.yaml,契约变更先改文档 | 用户:API 文档唯一标准 |

## 11. 实施步骤(执行期,按序)

1. **commit 当前改动**(见第 0 节)。
2. `web/CLAUDE.md`:规则文件(API 文档=唯一契约、变更顺序「先文档后代码」、Go/Vue 命名约定、错误码总表、指纹 canonical 规则、**CSV 入库模板与校验/排重规则、分配与 30 天独占期规则**、安全清单)。
3. `web/api/API.md` + `web/api/openapi.yaml`:按第 6/7 节全量接口(字段表/示例/错误码),含设备端 lookup/bind 完整报文、入库 multipart 示例、分配/收回示例。
4. `web/backend`:go.mod → config → store → fingerprint → inventory(CSV 导入/分配/回收/到期)→ auth → api(6 组 handler + 路由)→ main(含定时器);`go build ./...` 通过。
5. `web/frontend`:Vite 脚手架 → api 层 → 6 页面(商户页含分配/收回,设备页含导入)→ 路由守卫;`npm run build` 通过。
6. 冒烟验证(见第 12 节):直接用真实 `E:\congmingpay-print\设备证书.csv` 走一遍导入(验证后删除该演示库,不污染正式 data)。

## 12. 验证清单

- [ ] 当前改动已 commit(开工前)。
- [ ] `web/backend`:`go build ./...` + `go vet` 通过;`web/frontend`:`npm run build` 通过。
- [ ] 主工程零改动:`git status` 确认 internal/、main.go 等无变化(除第 0 节已提交的 4 个文件)。
- [ ] 入库:用真实 设备证书.csv 导入成功(imported=N);再导入同文件 → 整批 `4004` 且零写入;改坏表头 → `4002`;造行内重复 → `4003`;与库存部分重复 → 整批 `4004`(不部分入库)。
- [ ] 分配:库存 N 时分配 N 成功、分配 N+1 → `3006`;分配后设备页状态=已分配未激活、所属商户正确;收回后回库存。
- [ ] 到期:把某设备 allocated_at 改为 31 天前 → 触发(重启或等定时器)后回库存。
- [ ] 设备端:lookup(短商户号+指纹A+deviceInfo{osType:win,appVersion:1.2.0})→ 返回名下 available(验证长短号均命中);bind → bound 且返回 deviceSecret/productKey,设备页「平台/版本」列显示 "Windows 1.2.0";重复 bind 幂等刷新 lastSeen 与版本;重装模拟 lookup(指纹A)→ `boundDeviceName` 原设备;bind 他设备名 → 3003;指纹B 抢已被占设备 → 3005;指纹空 → 3004;商户号不存在 → 2003;无设备的商户 lookup → 3001。
- [ ] 权限:operator 访问 /accounts、/inventory → 1002;无 token → 1001。
- [ ] 前端:登录/401 跳转/新增商户/分配设备对话框(库存数+数量)/导入 CSV/设备只读总表/账号页仅 admin。
- [ ] API.md 与 openapi.yaml 与实现三者一致(以文档为准)。
