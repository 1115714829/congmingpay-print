# web 管理系统设计(打印服务器管理云端)

## 1. 摘要

新建 `web/` 目录,包含独立的 Go 后端(云端服务,可部署 Linux/Windows)与 Vue 前端(管理控制台)。核心能力:
1. **账号密码管理**:注册/登录/改密/管理员管账号(启停用、重置密码、改角色)。
2. **门店(DeviceName 归属)**:门店档案 + 设备(DeviceName)注册与门店归属绑定。
3. **设备指纹绑定/装系统恢复**:设备端(PC/安卓)上报硬件指纹与 DeviceName 形成绑定;重装系统后凭**恢复码**以新指纹重新绑定。
4. **API 接口文档**:`web/CLAUDE.md`(规则说明)+ `web/api/API.md`(人类可读)+ `web/api/openapi.yaml`(机器可读 OpenAPI 3.0)——三者是客户端(前端/Windows 主程序/安卓)与后端通信的唯一标准。

**已确认**:独立云端服务;本期只设计并实现 web 端,主程序不改造(设备上报接口在 API 文档中完整预留)。

## 2. 现状依据(已核实)

- 主程序:纯 Windows GUI、无 HTTP API、无账号体系、无硬件指纹代码(DeviceName 手填,[Settings.go](file:///e:/congmingpay-print/internal/model/Settings.go))。
- 主工程 module `congmingpay`(go 1.20,Windows-only:walk)。
- 已有纯 Go SQLite 驱动 `modernc.org/sqlite`(CGO-free)可复用经验;无 JWT/加密库(标准库 crypto 即可)。
- 无 `web/` 目录。

## 3. 目录与工程结构

```
web/
├── CLAUDE.md               # 规则说明:API 文档是唯一通信标准 + 开发约定
├── api/
│   ├── API.md              # 完整接口文档(唯一参考物,人读)
│   └── openapi.yaml        # OpenAPI 3.0 同份契约(机器可读,可导入 swagger)
├── backend/                # 独立 Go module:congmingpay/web (go 1.20,跨平台)
│   ├── go.mod
│   ├── main.go             # 启动:读 env → 初始化 DB/路由 → 起服务 → 种子管理员
│   ├── internal/
│   │   ├── config/         # 环境变量配置
│   │   ├── store/          # SQLite:建表、迁移、仓储(Account/Store/Device/Binding/Recovery/BindRequest/Log)
│   │   ├── auth/           # bcrypt 密码、JWT(HS256)签发/校验、中间件
│   │   ├── fingerprint/    # 指纹归一化与 SHA256 摘要
│   │   └── api/            # 路由 + handler:Login.go Store.go Device.go Binding.go Account.go
│   └── data/               # 运行时(web.db),gitignore
└── frontend/               # Vue 3 + Vite + Vue Router + Pinia + Element Plus + axios
    ├── package.json
    ├── vite.config.js      # /api 代理到后端端口
    └── src/
        ├── main.js
        ├── api/http.js     # axios 实例(统一错误处理 + Bearer 注入)
        ├── api/{auth,store,device,account}.js
        ├── stores/auth.js  # 登录态(token + 用户信息)
        ├── router/index.js # 路由 + 登录守卫
        └── views/{Login,Dashboard,Stores,Devices,Accounts,Profile}.vue
```

### 依赖决策
- **后端新增依赖**:`github.com/golang-jwt/jwt/v5`(JWT)、`golang.org/x/crypto`(bcrypt)、`modernc.org/sqlite`(数据库)、标准库 `net/http`(路由,不引框架)、`gopkg.in/yaml.v3` 不需要。
- **不引 Gin/Echo**:标准库 ServeMux 足够(路由量 ~20 条),保持轻。
- **前端**:`vue@3`、`vue-router@4`、`pinia`、`element-plus`、`axios`;构建 Vite 5。UI 中文,布局:登录页 + 主框架(左侧菜单:总览/门店/设备/账号/个人中心)。
- **web 后端为独立 module**(`congmingpay/web`),不依赖主工程 walk 代码,保证 Linux 部署。

## 4. 数据模型(SQLite)

```sql
accounts(id PK, username UNIQUE, password_hash, display_name,
         role TEXT CHECK(role IN ('admin','operator')), enabled INTEGER DEFAULT 1,
         created_at, updated_at)

stores(id PK, name UNIQUE, code, address, contact_phone, remark,
       created_by, created_at, updated_at)

devices(id PK, device_name UNIQUE,           -- = MQTT 物联网 DeviceName
        store_id NULL FK→stores,
        status TEXT DEFAULT 'pending',       -- pending 未归属 / bound 已绑定 / unbound 已解绑
        first_report_at, last_seen_at,
        created_at, updated_at)

device_bindings(id PK, device_id FK→devices UNIQUE,  -- 当前生效绑定(一设备一条)
        fingerprint TEXT,                    -- 归一化字段 JSON
        fingerprint_hash TEXT,               -- SHA256(fingerprint)
        source TEXT,                         -- 'initial' 首绑 / 'recovery' 恢复绑定
        recovered INTEGER DEFAULT 0,         -- 是否经历过重装恢复
        created_at)

recovery_codes(id PK, device_id FK→devices,
        code_hash TEXT,                      -- SHA256(code),码本身不落地
        expires_at, used INTEGER DEFAULT 0, created_at)

bind_requests(id PK, device_id, store_id NULL, fingerprint, source,
        status TEXT DEFAULT 'pending',       -- pending / approved / rejected
        review_note, reviewed_by, reviewed_at, created_at)

login_logs(id PK, username, ip, ua, ok INTEGER, reason, created_at)
```

规则:
- 设备按 `device_name` 自动建档(首见即插入,不要求先注册——设备先上线再归属)。
- 绑定状态机:`未绑定 →(指纹首报) 自动绑定` 或 `未绑定 →(已有绑定且指纹不同) 挂起待审`;`已绑定 → 管理员解绑 → 生成恢复码 →(恢复码+新指纹) 恢复绑定`。

## 5. 设备指纹与恢复(核心规则)

**指纹字段(设备端采集,上报)**:
| 字段 | PC | 安卓 |
|---|---|---|
| osType | `win` | `android` |
| boardSerial | 主板序列号 | 主板/平台序列号 |
| cpuId | CPU ID | 处理器型号 |
| diskSerials | 磁盘序列号数组 | 存储卷序列号 |
| macAddress | 主网卡 MAC | Wi-Fi MAC |
| osBuild / deviceModel | 系统版本 / — | 系统版本 / 机型 |
| appVersion | 主程序版本 | 应用版本 |

- `fingerprint_hash = SHA256(canonicalJSON)`,canonicalJSON = 字段按名排序、数组内部排序、TrimSpace 后序列化(大小写:MAC 大写、序列号保留原样)。
- **首报绑定**:device 无 binding 且无 pending request → 直接绑定(status=bound)。
- **冲突**:已有生效绑定且新指纹 hash 不同 → 不覆盖,生成 `bind_request`(pending),管理端设备页出现"待审核绑定",管理员批准(覆盖绑定,记 recovered 若来自恢复码)或驳回。
- **恢复流程**:
  1. 管理端(或设备凭旧指纹)对已绑定设备"生成恢复码":6 位大写字母数字,服务端存 SHA256,有效期 30 分钟,可重复生成(旧码作废)。
  2. 重装系统后设备端上报 `{deviceName, recoveryCode, 新指纹}` → 校验码有效且属该设备 → 覆盖绑定(source=recovery, recovered=1, 状态 bound)。
  3. 码过期/用错 → 明确错误码,设备端提示重新申请。

## 6. API 契约(唯一标准;完整定义在 API.md/openapi.yaml)

**通用约定**:
- Base:`/api/v1`;全部 JSON UTF-8;时间 RFC3339。
- 响应统一信封:`{"code":0,"message":"ok","data":...}`;`code=0` 成功,非 0 业务错误(HTTP 仍 200,仅 401/404/405 用状态码)。
- 鉴权:管理端接口 `Authorization: Bearer <JWT>`(HS256,24h);设备端接口 `X-Device-Name` + 指纹/恢复码。
- 错误码:`1001 未登录/令牌过期` `1002 无权限` `1003 用户名或密码错误` `1004 账号停用` `2001 参数错误` `2002 用户名已存在` `3001 设备未找到` `3002 恢复码无效或过期` `3003 指纹冲突待审核` `3004 已有待审核请求`。

**A. 账号与登录**(Bearer 区):
| 方法 路径 | 说明 |
|---|---|
| POST /api/v1/login | `{username,password}` → `{token, user{...}}`;写 login_logs |
| GET /api/v1/me | 当前账号信息 |
| PUT /api/v1/me/password | `{oldPassword,newPassword}` |
| GET /api/v1/accounts (admin) | 分页列表 `{page,pageSize}` |
| POST /api/v1/accounts (admin) | 新建 `{username,password,displayName,role}` |
| PUT /api/v1/accounts/{id} (admin) | 改 displayName/role/enabled |
| PUT /api/v1/accounts/{id}/reset-password (admin) | `{newPassword}` |

**B. 门店**(Bearer):
| 方法 路径 | 说明 |
|---|---|
| GET /api/v1/stores | 列表(含设备数统计) |
| POST /api/v1/stores | 新建 `{name,code,contactPhone,address,remark}` |
| PUT /api/v1/stores/{id} | 编辑 |
| DELETE /api/v1/stores/{id} | 删除(有归属设备则拒绝,code=2001) |
| PUT /api/v1/stores/{id}/devices/{deviceName} | 把设备归属到本门店(归属变更,不影响指纹绑定) |
| DELETE /api/v1/stores/{id}/devices/{deviceName} | 解除设备门店归属 |

**C. 设备与绑定**(Bearer):
| 方法 路径 | 说明 |
|---|---|
| GET /api/v1/devices | 列表,支持 `?storeName=&status=&keyword=` 过滤;返回绑定摘要 |
| GET /api/v1/devices/{deviceName} | 详情(绑定指纹、lastSeen、请求历史) |
| POST /api/v1/devices/{deviceName}/recovery-code (admin) | 生成恢复码 → `{code, expiresAt}`(仅此一次返回明文) |
| POST /api/v1/devices/{deviceName}/unbind (admin) | 解绑(指纹绑定作废,状态 unbound,保留档案) |
| GET /api/v1/bind-requests | 待审核绑定列表 |
| POST /api/v1/bind-requests/{id}/approve | 批准(覆盖绑定) |
| POST /api/v1/bind-requests/{id}/reject | 驳回 `{note}` |

**D. 设备端上报**(X-Device-Name,供 Windows 主程序/安卓后续实现):
| 方法 路径 | 说明 |
|---|---|
| POST /api/v1/device/report | `{fingerprint:{...}, appVersion}` → 自动首绑/心跳(lastSeen);冲突时返回 `3003` |
| POST /api/v1/device/recover | `{recoveryCode, fingerprint}` → 恢复绑定;成功返回 `{status:"bound"}` |

**E. 总览**:
| 方法 路径 | 说明 |
|---|---|
| GET /api/v1/dashboard | 门店数/设备数/已绑定数/待审核数/近 7 天登录成功失败数 |

**心跳/在线**:无独立心跳接口——`POST /device/report` 每次调用即心跳(设备端建议 5 分钟一次)。`lastSeenAt > now-120s` 视为在线(列表返回 computed `online` 布尔)。

## 7. 后端关键实现点

- `main.go`:env `WEB_PORT`(默认 9000)、`WEB_DATA`(默认 ./data)、`JWT_SECRET`(默认值 + 日志警告建议更换)、`ADMIN_USERNAME`(默认 admin)、`ADMIN_PASSWORD`(默认 admin123,**首次启动且 accounts 为空时种子创建,并日志强烈提示修改**)。
- 启动流程:open sqlite(pragma journal_mode=WAL, busy_timeout=5000)→ migrate(建表 IF NOT EXISTS)→ 种子管理员 → 注册路由 → `http.Server{ReadHeaderTimeout:5s}.ListenAndServe`;优雅退出(Signal → Shutdown 500ms,与主程序 docserver 同款模式)。
- 路由:标准库 `http.ServeMux`(Go1.20 无方法路由 → 自写 20 行 dispatch:按 `METHOD /path` 精确表 + 单段路径参数解析,不引框架)。
- 中间件链:CORS(开发期允许前端 vite 端口;生产同源无影响)→ 设备区(校验 X-Device-Name 非空)或 Bearer 区(校验 JWT、注入 user)→ handler。
- JWT:payload `{uid, username, role, exp}`,HS256;过期返回 1001。
- 密码:bcrypt cost 10;新用户密码校验 6-64 位含字母数字。
- 并发:所有 DB 访问走单连接 mutex 或 modernc 自带并发安全(sqlite WAL 多读单写,用 `database/sql` + SetMaxOpenConns(1) 保简单)。
- 日志:沿用主程序风格(文件 + 启动横幅),log 目录 `data/log`。

## 8. 前端页面与交互

- **登录页**:用户名/密码 → 存 token(Pinia + sessionStorage)→ 跳总览;401 全局拦截回登录页。
- **总览**:4 张统计卡(门店/设备/已绑定/待审核)+ 近 7 天登录柱状图(纯 Element 表格实现,不引 chart 库)。
- **门店页**:表格(名称/编码/联系人/设备数)+ 新建/编辑对话框;行操作"管理设备"(抽屉:设备搜索 + 勾选归属)。
- **设备页**:表格(DeviceName/所属门店/状态/在线/指纹摘要/最近上线)+ 过滤栏;行操作:详情(指纹 JSON 折叠展示)、生成恢复码(对话框展示码+过期时间+复制)、解绑(二次确认)。
- **待审核**(设备页 Tab 或顶部红点):待审核绑定列表,批准/驳回。
- **账号页**(admin 可见):列表 + 新建/编辑/重置密码/停用。
- **个人中心**:改密码。
- 统一 `http.js`:baseURL `/api/v1`、请求带 Bearer、响应 `code!=0` 时 ElMessage 报错并 reject。

## 9. 假设与决策

| 决策 | 取值 | 依据 |
|---|---|---|
| 部署形态 | 独立云端服务,web 后端可跑 Linux | 用户确认 |
| 主程序 | 本期不改,设备上报接口完整预留于 API 文档 | 用户确认 |
| web 工程 | 独立 Go module `congmingpay/web`,go 1.20 | 不拖入 walk 的 Windows-only 依赖 |
| 路由/JWT/DB | 标准库路由 + golang-jwt/v5 + modernc sqlite | 轻量、无 CGO、跨平台 |
| 会话 | 单 JWT 24h,无 refresh | 管理后台场景够用 |
| 设备鉴权 | 首绑无密钥(局域网设备+DeviceName 即身份);恢复用 30min 一次性码;冲突走人工审核 | 防止任意设备冒名覆盖绑定 |
| 默认管理员 | admin/admin123,首次启动种子 + 启动日志强提示修改 | 便于交付,安全靠提示+可改 |
| 文档 | CLAUDE.md(规则) + API.md + openapi.yaml,契约变更必须先改文档 | 用户要求 API 文档为唯一标准 |

## 10. 实施步骤(执行期)

1. 建 `web/CLAUDE.md`(规则:文档即契约、变更顺序、命名、错误码表、指纹 canonical 规则、安全清单)。
2. 写 `web/api/API.md` + `web/api/openapi.yaml`(按第 6 节全量字段/示例)。
3. `web/backend`:go.mod → config → store(建表/迁移/仓储)→ auth → fingerprint → 5 组 handler + 路由 + main + 种子;`go build` 通过,手工 curl 冒烟(登录→建门店→设备首报→冲突→恢复码→恢复)。
4. `web/frontend`:Vite 脚手架 → api 层 → 6 个页面 → 登录守卫;`npm run dev` 联调。
5. 端到端验证:curl 模拟设备首绑/重装恢复;浏览器走登录/门店归属/生成恢复码/审核全流程;后端重启数据不丢。
6. 更新根 README 不要求(除非用户后续要);git 提交(用户要求时)。

## 11. 验证清单

- [ ] `cd web/backend && go build ./...` 通过;`cd web/frontend && npm run build` 通过。
- [ ] curl:admin 登录拿 token;未带 token 访问 /stores 返回 1001。
- [ ] curl 设备区:`POST /device/report` 首报成功(status=bound);同 DeviceName 换指纹再报返回 3003 且生成待审核;管理端批准/驳回两条路径均可。
- [ ] 恢复:生成恢复码 → `POST /device/recover` 成功(status=bound,recovered=1);过期码返回 3002。
- [ ] 门店归属:设备归属变更后列表正确;删除有设备的门店被拒。
- [ ] 前端:登录失败提示、401 跳登录、admin/operator 菜单差异(账号页仅 admin)、恢复码复制可用。
- [ ] API.md 与 openapi.yaml 与实现三者一致(以文档为准,实现不符即改实现)。
