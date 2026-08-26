# 打印服务器 SN/DeviceName 管理中心 — API 接口文档

> **本文档是前端(Vue)与后端(Go)通信的唯一参考标准。** 字段/错误码以本文为准;
> 机器可读版见 [openapi.yaml](openapi.yaml)(与本文一一对应)。变更流程见 `../CLAUDE.md` 第 9 节。

## 0. 通用约定

- Base URL:`/api/v1`(开发期前端经 Vite 代理到 `http://localhost:9000`)
- 请求/响应体:JSON,UTF-8;时间格式 RFC3339(如 `2026-08-26T10:00:00+08:00`)
- 响应信封(所有接口统一):

```json
{ "code": 0, "message": "ok", "data": { } }
```

  - `code=0` 成功;非 0 为业务错误(此时 HTTP 状态码仍为 200)。
  - HTTP 状态码仅用于:401 未带/无效令牌(管理端)、404 路径不存在、405 方法不允许。
- 管理端鉴权:`Authorization: Bearer <JWT>`(HS256,有效期 24h)。
- 设备端接口(第 6 组):`X-Device-Name` 请求头可空(仅日志);身份在 body 指纹里。
- 角色:`admin`(全量权限);`operator`(不可访问账号管理、入库、解绑、批量解绑、批量回收、删除商户 → 返回 `1002`)。

### 错误码总表

| code | message | 说明 |
|---|---|---|
| 0 | ok | 成功 |
| 1001 | 未登录或令牌过期 | 管理端无 token/token 无效/过期 |
| 1002 | 无权限(需admin) | operator 访问 admin-only 接口 |
| 1003 | 用户名或密码错误 | 登录失败 |
| 1004 | 账号已停用 | 停用账号登录 |
| 2001 | 参数错误 | 必填缺失/格式非法/业务不允许(如收回已绑定设备) |
| 2002 | 用户名已存在 | 新建账号 |
| 2003 | 商户号不存在 | 设备端 lookup/bind 的 merchantNo 长/短均未命中 |
| 2004 | 商户号(长/短)已存在 | 新增商户,长号或短号与他人重复 |
| 3001 | 该商户暂无可认领设备 | 未分配设备,或分配已过期回库存 |
| 3002 | DeviceName不存在或不属于该商户 | bind 目标设备不属于该商户(含已过期回库存) |
| 3003 | 该设备(SN)已绑定其他DeviceName | 指纹已绑 A,又请求绑 B |
| 3004 | 指纹无效 | boardSerial/diskSerials 全空 |
| 3005 | 设备已被占用(抢注失败) | 并发 bind 同一 DeviceName,仅一个成功 |
| 3006 | 库存不足 | 分配数量 > 库存 |
| 4001 | 文件读取失败 | 上传读取错误 |
| 4002 | 模板格式错误 | CSV 表头/列数不符(带行号) |
| 4003 | 文件内DeviceName重复 | 同一 CSV 内重复(带行号) |
| 4004 | 与库存DeviceName重复 | 与库中已有设备重复(带行号,整批拒绝) |

### 公共对象

**User**(账号,密码永不下发):
```json
{
  "id": 1,
  "username": "admin",
  "displayName": "超级管理员",
  "role": "admin",            // admin | operator
  "enabled": true,
  "createdAt": "2026-08-26T10:00:00+08:00"
}
```

**Merchant**(商户):
```json
{
  "id": 1,
  "merchantNoLong": "M001122334",
  "merchantNoShort": "M001",
  "name": "示例门店",
  "contactPhone": "13800000000",
  "address": "",
  "remark": "",
  "allocatedCount": 2,        // 名下设备总数(未绑定+已绑定)
  "boundCount": 1             // 名下已绑定数
}
```

**Device**(设备;设备页表格列的字段来源):
```json
{
  "name": "ZYDY000000000026",
  "deviceSecret": "e62b4a826ff47c02cff2359f1f3dae0f",
  "productKey": "k02m26Wpp9S",
  "merchantId": 1,
  "merchantNoLong": "M001122334",
  "merchantNoShort": "M001",
  "merchantName": "示例门店",
  "state": "allocated",       // inventory 库存 | allocated 已分配未激活 | bound 已绑定
  "allocatedAt": "2026-08-26T10:00:00+08:00",
  "allocatedLeftDays": 25,    // 未激活时:独占期剩余天数(0~30);其他状态为 0
  "boundAt": null,
  "lastSeenAt": "2026-08-26T12:00:00+08:00",
  "online": true,             // lastSeenAt 距今 < 120s
  "osType": "win",            // win | android;未上报为 null
  "appVersion": "1.2.0"       // 未上报为 null
}
```

**Fingerprint**(设备指纹,身份字段 4 个 + 展示字段 3 个):
```json
{
  "osType": "win",            // 必填: win | android(参与摘要)
  "boardSerial": "SN-MAIN-001",// 参与摘要
  "cpuId": "BFEBFBFF000906EA", // 参与摘要
  "diskSerials": ["WD-WCC6Y..."],    // 参与摘要(系统盘序列号数组,内部排序)
  "osBuild": "10.0.19045",    // 仅展示,不参与摘要
  "appVersion": "1.2.0",      // 仅展示,不参与摘要
  "deviceModel": null          // 仅展示(安卓机型/PC 可选)
}
```
> 摘要规则(参与摘要字段的 canonical):字段按名排序 + TrimSpace + diskSerials 排序 →
> SHA256。boardSerial/diskSerials 至少一项非空,否则 `3004`。
> 不收网卡 MAC(网卡不稳定、USB 网卡干扰);`diskSerials` 只含系统盘(避免多盘干扰)。

**DeviceInfo**(设备端上报的设备信息,lookup/bind 携带):
```json
{ "osType": "win", "appVersion": "1.2.0", "osBuild": "10.0.19045", "deviceModel": "DESKTOP" }
```
> `osType` 必填("win"/"android");其余可选。每次 lookup/bind 都会刷新对应设备的
> `os_type/app_version`(设备页「平台/版本」列来源)。

---

## 1. 账号与登录

### 1.1 登录
`POST /api/v1/login`(无需 token)

请求:
```json
{ "username": "admin", "password": "admin123" }
```
响应 `data`:
```json
{
  "token": "eyJhbGciOiJIUzI1NiIs...",
  "user": { "id": 1, "username": "admin", "displayName": "超级管理员", "role": "admin", "enabled": true, "createdAt": "..." }
}
```
错误:`1003` 用户名或密码错误;`1004` 账号已停用。登录成败均写 login_logs。

### 1.2 当前账号
`GET /api/v1/me`(Bearer)
响应 `data`:User。

### 1.3 修改自己密码
`PUT /api/v1/me/password`(Bearer)
请求:`{ "oldPassword": "admin123", "newPassword": "NewPass123" }`
错误:`1003` 原密码错误;`2001` 新密码不合法(6-64 位且含字母数字)。

### 1.4 账号列表 (admin)
`GET /api/v1/accounts?page=1&pageSize=20&keyword=xx`(Bearer)
响应 `data`:
```json
{ "total": 3, "items": [ User... ] }
```

### 1.5 新建账号 (admin)
`POST /api/v1/accounts`(Bearer)
请求:`{ "username": "op01", "password": "Passw0rd", "displayName": "运营", "role": "operator" }`
- username 仅字母数字下划线 2-32 位;role:`admin`|`operator`。
响应 `data`:User。错误:`2002` 用户名已存在;`2001` 参数错误。

### 1.6 编辑账号 (admin)
`PUT /api/v1/accounts/{id}`(Bearer)
请求(字段可缺省,缺省不改):`{ "displayName": "运营组长", "role": "operator", "enabled": false }`
响应 `data`:User。

### 1.7 重置密码 (admin)
`PUT /api/v1/accounts/{id}/reset-password`(Bearer)
请求:`{ "newPassword": "NewPass123" }` → 响应 `data: null`。

---

## 2. 商户(新增/删除)

### 2.1 商户列表
`GET /api/v1/merchants`(Bearer)
响应 `data`:
```json
{ "items": [ Merchant... ] }
```

### 2.2 新增商户(新增完成即入库)
`POST /api/v1/merchants`(Bearer)
请求:
```json
{
  "merchantNoLong": "M001122334",
  "merchantNoShort": "M001",
  "name": "示例门店",
  "contactPhone": "13800000000",
  "address": "杭州市西湖区",
  "remark": ""
}
```
- `merchantNoLong`/`merchantNoShort`/`name` 必填,仅字母数字下划线 1-32 位(名称可中文);
  长号、短号分别全局唯一。
响应 `data`:Merchant。错误:`2004` 商户号(长/短)已存在;`2001` 参数错误。

### 2.3 商户详情
`GET /api/v1/merchants/{id}`(Bearer)
响应 `data`:Merchant(同列表项)。

### 2.4 删除商户 (admin)
`DELETE /api/v1/merchants/{id}`(Bearer,admin)
- 名下**有已绑定设备**时拒绝(`2001`,须先解绑);
  名下"已分配未绑定"设备随删除自动回库存(清商户归属与分配时间)。
响应 `data`:
```json
{ "released": 40 }
```
错误:`2001` 商户不存在/有已绑定设备;`1002` operator 无权限。

---

## 3. 商户设备(分配/收回)

### 3.1 分配设备(从库存提取)
`POST /api/v1/merchants/{id}/allocate`(Bearer)
请求:`{ "count": 2 }`
- 从库存按入库顺序取 count 个,置为该商户独占(30 天未激活自动回库存)。
响应 `data`:
```json
{ "allocated": ["ZYDY000000000026", "ZYDY000000000048"] }
```
错误:`3006` 库存不足(此时一个都不分配,事务回滚);`2001` count 非法(≤0)。

### 3.2 名下设备列表
`GET /api/v1/merchants/{id}/devices`(Bearer)
响应 `data`:
```json
{ "items": [ Device... ] }
```

### 3.3 收回设备(回库存)
`POST /api/v1/merchants/{id}/devices/{name}/reclaim`(Bearer)
- 仅"已分配未绑定"可收回;已绑定返回 `2001`(需先解绑)。
响应 `data: null`。错误:`3002` 设备不属于该商户。

---

## 4. 设备(总表/详情/解绑/批量解绑/批量回收)

### 4.1 设备总表
`GET /api/v1/devices?page=1&pageSize=20&keyword=&merchantId=&state=`(Bearer)
- `keyword`:匹配 DeviceName/商户号/商户名(可空);`state`:`inventory`|`allocated`|`bound`(可空)。
响应 `data`:
```json
{ "total": 42, "items": [ Device... ] }
```

### 4.2 设备详情
`GET /api/v1/devices/{name}`(Bearer)
响应 `data`:
```json
{
  "name": "ZYDY000000000026",
  "deviceSecret": "e62b4a826ff47c02cff2359f1f3dae0f",
  "productKey": "k02m26Wpp9S",
  "merchant": { "id": 1, "merchantNoLong": "M001122334", "merchantNoShort": "M001", "name": "示例门店" },
  "state": "bound",
  "allocatedAt": "2026-08-26T10:00:00+08:00",
  "boundAt": "2026-08-26T12:00:00+08:00",
  "lastSeenAt": "2026-08-26T12:05:00+08:00",
  "online": true,
  "osType": "win",
  "appVersion": "1.2.0",
  "fingerprint": Fingerprint   // 绑定时的原始指纹 raw(未绑定为 null)
}
```
错误:`3002` 设备不存在。

### 4.3 解绑 (admin)
`POST /api/v1/devices/{name}/unbind`(Bearer,admin)
- 换硬件/回收场景:清指纹绑定并解除商户归属,设备回库存。仅已绑定可解绑,否则 `2001`。
响应 `data: null`。

### 4.4 批量解绑 (admin)
`POST /api/v1/devices/batch-unbind`(Bearer,admin)
请求:
```json
{ "names": ["ZYDY000000000026", "ZYDY000000000048"] }
```
- `names` 1-200 个(自动去重);逐台解绑,单台失败不影响其余。
响应 `data`:
```json
{ "unbound": ["ZYDY000000000026"], "skipped": { "ZYDY000000000048": "未绑定" } }
```
- `skipped` 原因取值:`未绑定` / `设备不存在`。
错误:`2001` 参数错误;`1002` operator 无权限。

### 4.5 批量回收 (admin)
`POST /api/v1/devices/batch-reclaim`(Bearer,admin)
请求:
```json
{ "names": ["ZYDY000000000026", "ZYDY000000000048"] }
```
- `names` 1-200 个(自动去重);把「已分配未激活」设备收回库存(清商户归属与分配时间),逐台执行,单台失败不影响其余。
响应 `data`:
```json
{ "reclaimed": ["ZYDY000000000026"], "skipped": { "ZYDY000000000048": "已绑定" } }
```
- `skipped` 原因取值:`已绑定` / `未分配` / `设备不存在`。
错误:`2001` 参数错误;`1002` operator 无权限。

---

## 5. 入库(CSV 导入/库存列表)(admin)

### 5.1 导入设备(CSV)
`POST /api/v1/inventory/import`(Bearer,admin)
`Content-Type: multipart/form-data`,文件字段名 **`file`**,UTF-8(允许 BOM)。
模板(三列,首行为表头):
```csv
DeviceName,DeviceSecret,ProductKey
ZYDY000000000026,e62b4a826ff47c02cff2359f1f3dae0f,k02m26Wpp9S
ZYDY000000000048,da2fc1ab09a4f34b96d13c97e251532b,k02m26Wpp9S
```
校验(任一失败 → **整批拒绝、零写入、事务回滚**,message 带首个出错行号):
- 表头必须恰为 `DeviceName,DeviceSecret,ProductKey`(列首尾空格忽略)→ `4002`;
- 每行三列非空;DeviceName 仅字母数字 1-64;DeviceSecret/ProductKey 1-128 → `4002`;
- 文件内 DeviceName 重复 → `4003`;
- 与库中已有 DeviceName 重复(库存/已分配/已绑定)→ `4004`。

响应 `data`:
```json
{ "imported": 42, "total": 42 }
```

### 5.2 库存列表
`GET /api/v1/inventory?page=1&pageSize=50`(Bearer,admin)
响应 `data`:
```json
{ "total": 10, "items": [ Device...(均为库存态) ] }
```

---

## 6. 设备端(Windows 主程序/安卓客户端;无 JWT,凭商户号+硬件指纹)

> 这两个接口是**核心功能**:设备端点「获取设备信息」→ 读硬件指纹 → 输商户号 → lookup →
> 选择 DeviceName(密钥随 lookup 下发,写入本地 MQTT 配置)→ 用户「保存设置」生效后才 bind 上报绑定确认。
> lookup 兼心跳:客户端建议每 5 分钟调一次 lookup(或 bind)。

### 6.1 查询可认领设备 / 重装恢复探测
`POST /api/v1/device/lookup`(头 `X-Device-Name` 可空)
请求:
```json
{
  "merchantNo": "M001",
  "fingerprint": Fingerprint,
  "deviceInfo": { "osType": "win", "appVersion": "1.2.0", "osBuild": "10.0.19045" }
}
```
响应 `data`(正常):
```json
{
  "merchant": { "merchantNoLong": "M001122334", "merchantNoShort": "M001", "name": "示例门店" },
  "boundDevice": null,
  "availableDevices": [
    { "name": "ZYDY000000000026", "productKey": "k02m26Wpp9S", "deviceSecret": "e62b…" }
  ],
  "total": 2,
  "boundCount": 0
}
```
`boundDevice`／`availableDevices` 均为 `{name, productKey, deviceSecret}`(设备源地址+商户号即可取到密钥,
与 bind 返回一致;客户端据此在本地完成 MQTT 配置回填)。
**重装恢复场景**:该指纹已绑定过某 DeviceName 时,`boundDevice` 返回该设备(名称+密钥)
(其余字段仍按当前商户返回;客户端提示「已绑定 X,不能切换」,保存设置后对 X 调 bind 恢复)。
**绑定时序**:lookup 只查询不绑定;客户端选中 SN 后点「保存设置」,设置生效时才调 bind 上报确认。
错误:`2003` 商户号不存在;`3004` 指纹无效;
`3001` 该商户暂无可认领设备(仅当 `boundDevice` 为空且 `availableDevices` 为空时返回,HTTP 仍 200 但 code=3001,data 同上结构)。
副作用:刷新该指纹/设备的 `last_seen_at`;若指纹已绑定设备,顺带刷新其 `os_type/app_version`。

### 6.2 绑定(认领/恢复,幂等兼心跳)
`POST /api/v1/device/bind`(头 `X-Device-Name` 可空)
请求:
```json
{
  "merchantNo": "M001",
  "deviceName": "ZYDY000000000026",
  "fingerprint": Fingerprint,
  "deviceInfo": { "osType": "win", "appVersion": "1.2.0", "osBuild": "10.0.19045" }
}
```
响应 `data`(**含密钥,供客户端直接写入本地 MQTT 配置**):
```json
{
  "deviceName": "ZYDY000000000026",
  "deviceSecret": "e62b4a826ff47c02cff2359f1f3dae0f",
  "productKey": "k02m26Wpp9S",
  "status": "bound"
}
```
规则:
- DeviceName 必须属于该商户且未绑定 → 原子置绑定(并发抢注仅一个成功);
- 指纹已绑其他 DeviceName 且请求不同 → `3003`(message 中返回已绑定的设备名,客户端应改绑回去);
- 同一指纹重复 bind 同一 DeviceName → 幂等成功,刷新 last_seen_at 与 os_type/app_version。
错误:`2003` 商户号不存在;`2001` deviceName 缺失;`3002` 设备不存在/不属于该商户/已过期回库存;
`3003` 指纹已绑其他设备;`3004` 指纹无效;`3005` 设备已被占用。

---

## 7. 总览

### 7.1 仪表盘统计
`GET /api/v1/dashboard`(Bearer)
响应 `data`:
```json
{
  "merchantCount": 5,
  "inventoryCount": 10,
  "allocatedCount": 6,
  "boundCount": 26,
  "loginStats": [
    { "date": "2026-08-20", "success": 4, "failed": 1 }
  ]
}
```
`loginStats` 为近 7 天(含今天)按日统计。
