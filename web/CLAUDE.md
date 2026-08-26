# CLAUDE.md — web 管理系统规则文件

> 本文件是 `web/` 目录的开发规则说明。**`web/api/API.md`(及其机器可读版 `web/api/openapi.yaml`)
> 是客户端前端(Vue)与后端(Go)之间通信的唯一参考标准。**
> 任何接口/字段/错误码的变更:先改 API.md,再改 openapi.yaml,最后改代码;实现与文档不一致时以文档为准。

## 1. 系统定位

- 打印服务器的 SN/DeviceName 管理中心(独立云端服务,可部署 Linux/Windows)。
- 核心链路:设备端(Windows 主程序/安卓客户端)读取本机**硬件指纹** → 输入**商户号**(长或短)
  → `POST /api/v1/device/lookup` 拿到该商户名下可认领的 DeviceName+密钥(或重装后直接返回原绑定),
  客户端仅**回填本地 MQTT 配置、不绑定** → 用户点「保存设置」生效后才 `POST /api/v1/device/bind`
  建立 **1 DeviceName ↔ 1 硬件指纹** 的唯一绑定(幂等兼心跳)。
- **重装系统不变**:身份是硬件指纹(主板序列号/CPU ID/系统盘序列号),不是本地文件;
  重装后同硬件 → 同指纹 → lookup 直接返回原绑定,「保存设置」后一键恢复。
- 打印机上报/MQTT/点餐系统通信属于主工程(internal/mqtt、internal/printsvc 等),
  **与本系统无关,本目录不得修改主工程那些代码**。

## 2. 目录与工程约定

```
web/
├── CLAUDE.md          # 本文件
├── api/               # 契约文档(API.md + openapi.yaml)——唯一通信标准
├── backend/           # Go 独立 module:congmingpay/web(go 1.20,零 CGO,Linux 可部署)
│   ├── main.go        # 启动:env → 打开 DB → 迁移 → 种子管理员 → 到期回收定时器 → 路由
│   └── internal/      # config / store / auth / fingerprint / inventory / api
└── frontend/          # Vue 3 + Vite + vue-router + pinia + element-plus + axios
```

- 后端是**独立 Go module**(`congmingpay/web`),不 import 主工程任何包(主工程依赖 walk,仅 Windows)。
- 只允许新增依赖:`golang-jwt/jwt/v5`、`golang.org/x/crypto`(bcrypt)、`modernc.org/sqlite`(纯 Go)。
  路由用标准库 `net/http`,不引 Gin/Echo 等框架。
- Go 命名沿用主工程:包名小写、导出标识符驼峰、文件名 PascalCase(如 `Merchant.go`)。
- 前端文件:视图 PascalCase(`Merchants.vue`),api/store 小写驼峰(`http.js`/`auth.js`)。
- 运行数据(后端 `data/` 下的 web.db、log)不得提交 git。

## 3. API 总则(与 API.md 一致)

- Base:`/api/v1`;JSON UTF-8;时间 RFC3339。
- 响应信封:`{"code":0,"message":"ok","data":...}`;`code=0` 成功;业务错误 HTTP 200 + 业务码;
  仅未登录(401)/路径不存在(404)/方法不允许(405)用 HTTP 状态码。
- 管理端鉴权:`Authorization: Bearer <JWT>`(HS256,24h,payload `{uid,username,role,exp}`)。
- 设备端接口:`X-Device-Name` 头可空(仅日志用);身份在 body 的指纹里。
- 角色:`admin`(全量)、`operator`(账号页/入库/解绑/批量解绑/删除商户不可用,返回 `1002`)。

### 错误码总表

| 码 | 含义 |
|---|---|
| 1001 | 未登录/令牌过期 |
| 1002 | 无权限(需 admin) |
| 1003 | 用户名或密码错误 |
| 1004 | 账号已停用 |
| 2001 | 参数错误 |
| 2002 | 用户名已存在 |
| 2003 | 商户号不存在 |
| 2004 | 商户号(长/短)已存在 |
| 3001 | 该商户暂无可认领设备(未分配或已过期回收) |
| 3002 | DeviceName 不存在或不属于该商户(含已过期回库存) |
| 3003 | 该设备(SN)已绑定其他 DeviceName |
| 3004 | 指纹无效 |
| 3005 | 设备已被占用(抢注失败) |
| 3006 | 库存不足 |
| 4001 | 文件读取失败 |
| 4002 | 模板格式错误(CSV 表头/列数) |
| 4003 | 文件内 DeviceName 重复 |
| 4004 | 与库存 DeviceName 重复 |

## 4. 设备指纹规则(身份规则,不得随意变更)

- **参与摘要(身份)的字段**——必须跨重装不变,全部硬件级:
  - `osType`:`"win"` / `"android"`
  - `boardSerial`:主板序列号
  - `cpuId`:CPU ID
  - `diskSerials`:**系统盘**序列号数组(**内部排序**;不收网卡 MAC——网卡不稳定、USB 网卡干扰;不收多盘——避免干扰)
- **不参与摘要(仅展示/记录)**:`osBuild`、`appVersion`、`deviceModel`。
- 摘要算法:`fp_hash = SHA256(canonicalJSON)`;canonicalJSON 规则:
  1. 只含上面 4 个身份字段;
  2. 字段按名**字典序**排序输出;
  3. 每个字符串值 TrimSpace;`diskSerials` 排序后以数组输出;
  4. 空值输出 `""`/`[]`,不得省略 key。
- 合法性:boardSerial / diskSerials **二者至少一项非空**,否则 `3004`。
- 设备端采集来源(PC):WMI `Win32_BaseBoard.SerialNumber`、`Win32_Processor.ProcessorId`、
  系统盘序列号(`Win32_OperatingSystem.SystemDeviceName` 定位系统所在盘 → `Win32_DiskDrive.SerialNumber`);
  安卓:`Build.BOARD`/平台序列号等。设备端实现不在本目录,本节仅作文档约定。
- 指纹 raw JSON 全量存 `fingerprint_bindings.raw`(供排查),查询只用 `fp_hash`。

## 5. 设备入库规则(CSV,强校验,整批原子)

- 模板(三列,即现有 `E:\congmingpay-print\设备证书.csv`):

  ```csv
  DeviceName,DeviceSecret,ProductKey
  ZYDY000000000026,e62b4a826ff47c02cff2359f1f3dae0f,k02m26Wpp9S
  ```

- 校验规则(任一失败 → **整批拒绝、零写入、事务回滚**):
  1. 文件 UTF-8(允许 BOM)/CSV,首行表头必须恰为 `DeviceName,DeviceSecret,ProductKey`
     (忽略列首尾空格)→ 否则 `4002 模板格式错误`;
  2. 每行三列均非空;DeviceName 仅字母数字、长度 1-64;DeviceSecret/ProductKey 长度 1-128;
  3. 文件内 DeviceName 重复 → `4003 文件内DeviceName重复`(带行号);
  4. 与库中已有 DeviceName(库存+已分配+已绑定)重复 → `4004 与库存DeviceName重复`(带行号);
  5. 数据行不得为空文件。
- 成功:整批入库为**库存**(`merchant_id=NULL`),响应 `{imported, total}`。
- 写库约定:入库模块(当前=web 后端 API;将来若有独立入库程序)只写
  `device_name/device_secret/product_key/created_at` 四列,其余列保持 NULL/默认,
  不得写 `merchant_id/allocated_at/bound_fp_hash/bound_at/os_type/app_version`。

## 6. 分配与独占期规则

- **库存** = `merchant_id IS NULL` 的设备。
- **分配**:`POST /merchants/{id}/allocate {count}` 从库存按 `created_at ASC` 取 count 个,
  置 `merchant_id/allocated_at=now`,**商户独占 30 天**;取不满 → 事务回滚,`3006 库存不足`。
- **独占期到期**:启动即跑一次 + 每 1 小时定时:`allocated_at < now-30d 且 bound_fp_hash IS NULL`
  的设备清 `merchant_id/allocated_at` 回库存(纯 SQL 条件更新,幂等)。
- **收回**:管理端可对"已分配未绑定"设备立即收回回库存;已绑定的不得收回(需先解绑)。
- **认领(bind)**:设备端选择 DeviceName 绑定:
  - 设备必须属于该商户且未绑定 → `UPDATE ... WHERE device_name=? AND merchant_id=? AND bound_fp_hash IS NULL`,
    rowsAffected=0 → `3005`(并发抢注,只有一个成功);
  - 指纹已绑定其他 DeviceName 且选择的不是它 → `3003`;
  - 同一指纹重复 bind 同一 DeviceName → 幂等成功,刷新 `last_seen_at`(兼心跳)与 `os_type/app_version`。
- 绑定后设备页展示:设备SN(DeviceName)/设备密钥(DeviceSecret)/通信密钥(ProductKey)/
  所属商户/分配时间/平台版本(osType+appVersion)/状态/最近上线。

## 7. 商户规则

- 商户有**长商户号 `merchant_no_long` / 短商户号 `merchant_no_short`**,两者各自全局唯一;
  设备端 `merchantNo` 匹配顺序:先 long 精确匹配,未中再 short 精确匹配。
- 商户管理页提供**新增与删除**(新增完成即入库);删除时名下有已绑定设备则拒绝(须先解绑),
  未绑定(已分配)设备随删除自动回库存;本期不提供编辑。
- 商户号本期不与任何外部系统关联。

## 8. 账号与安全清单

- 密码 bcrypt(cost 10);新建账号密码 6-64 位且含字母数字。
- 首次启动且 accounts 为空时种子创建管理员(env `ADMIN_USERNAME`/`ADMIN_PASSWORD`,默认 admin/admin123),
  启动日志必须强提醒修改默认密码。
- `JWT_SECRET` 缺省用内置值时启动日志强提醒设置环境变量。
- 登录成败均写 `login_logs`(IP/UA/原因);管理端接口一律校验 Bearer;
  admin-only 接口(operator 访问)返回 `1002`。
- 默认管理员、JWT 24h、CORS 开发期仅放行 `http://localhost:5173`。

## 9. 变更流程(必须遵守)

1. 改 `web/api/API.md`(人读契约);
2. 同步改 `web/api/openapi.yaml`(机器读,与 API.md 一一对应);
3. 再改 `backend/` / `frontend/` 实现;
4. `go build ./...` 与 `npm run build` 通过;
5. 用 API.md 中的示例报文冒烟对应接口。
