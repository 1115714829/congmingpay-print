$ErrorActionPreference = 'Stop'
$port = if ($env:WEB_PORT) { $env:WEB_PORT } else { '9000' }
$base = "http://localhost:$port/api/v1"
$csv  = 'E:\congmingpay-print\web\certs.csv'
function Json($r){ if($r -is [string]){ $r | ConvertFrom-Json } else { $r } }

Write-Host '===== 1. 登录 admin ====='
$login = Invoke-RestMethod -Method Post -Uri "$base/login" -ContentType 'application/json' `
  -Body (@{username='admin';password='admin123'} | ConvertTo-Json)
$tok = $login.data.token
$h = @{ Authorization = "Bearer $tok" }
Write-Host ("token ok, user=" + $login.data.user.username + " role=" + $login.data.user.role)

Write-Host '===== 2. 新建商户 M001122334/M001 ====='
$mc = Invoke-RestMethod -Method Post -Uri "$base/merchants" -Headers $h -ContentType 'application/json' `
  -Body (@{merchantNoLong='M001122334';merchantNoShort='M001';name='示例门店';contactPhone='13800000000';address='北京';remark='smoke'} | ConvertTo-Json)
Write-Host ("merchant id=" + $mc.data.id + " long=" + $mc.data.merchantNoLong)

Write-Host '===== 3. CSV 导入(真实模板) ====='
$import = (& curl.exe -s -X POST "$base/inventory/import" -H "Authorization: Bearer $tok" -F "file=@$csv" 2>&1) -join ' '
Write-Host $import

Write-Host '===== 4. 重复导入(应 4004 整批拒绝) ====='
$dup = (& curl.exe -s -X POST "$base/inventory/import" -H "Authorization: Bearer $tok" -F "file=@$csv" 2>&1) -join ' '
Write-Host $dup

Write-Host '===== 5. 分配 2 台给商户 ====='
$alloc = Invoke-RestMethod -Method Post -Uri "$base/merchants/1/allocate" -Headers $h -ContentType 'application/json' -Body (@{count=2} | ConvertTo-Json)
Write-Host ("allocated=" + ($alloc.data.allocated -join ','))

Write-Host '===== 5b. 再分配 2 台(供批量回收) ====='
$alloc2 = Invoke-RestMethod -Method Post -Uri "$base/merchants/1/allocate" -Headers $h -ContentType 'application/json' -Body (@{count=2} | ConvertTo-Json)
Write-Host ("allocated=" + ($alloc2.data.allocated -join ','))

Write-Host '===== 6. 库存不足(分配 999 应 3006) ====='
$ins = Invoke-RestMethod -Method Post -Uri "$base/merchants/1/allocate" -Headers $h -ContentType 'application/json' -Body (@{count=999} | ConvertTo-Json)
Write-Host ("code=" + $ins.code + " msg=" + $ins.message)

$fp  = @{osType='win';boardSerial='B0ARD-1234';diskSerials=@('WD-AAA')}
$fp2 = @{osType='win';boardSerial='B0ARD-9999';diskSerials=@('WD-CCC')}

Write-Host '===== 7. lookup(短商户号 M001) ====='
$lk = Invoke-RestMethod -Method Post -Uri "$base/device/lookup" -ContentType 'application/json' `
  -Body (@{merchantNo='M001';fingerprint=$fp;deviceInfo=@{osType='win';appVersion='1.2.0';osBuild='10.0.19045'}} | ConvertTo-Json -Depth 5)
$availNames = @(); if ($lk.data.availableDevices) { $availNames = @($lk.data.availableDevices | ForEach-Object { $_.name }) }
Write-Host ("bound=" + $(if ($lk.data.boundDevice) { $lk.data.boundDevice.name } else { '<null>' }) + " avail=" + ($availNames -join ','))

Write-Host '===== 8. bind 第一台 ====='
$dev1 = $availNames[0]
$bd = Invoke-RestMethod -Method Post -Uri "$base/device/bind" -ContentType 'application/json' `
  -Body (@{merchantNo='M001';deviceName=$dev1;fingerprint=$fp;deviceInfo=@{osType='win';appVersion='1.2.0';osBuild='10.0.19045'}} | ConvertTo-Json -Depth 5)
Write-Host ("bound=" + $bd.data.deviceName + " secret=" + $bd.data.deviceSecret + " pk=" + $bd.data.productKey)

Write-Host '===== 9. 重复 bind 幂等 ====='
$bd2 = Invoke-RestMethod -Method Post -Uri "$base/device/bind" -ContentType 'application/json' `
  -Body (@{merchantNo='M001';deviceName=$dev1;fingerprint=$fp;deviceInfo=@{osType='win';appVersion='1.2.1'}} | ConvertTo-Json -Depth 5)
Write-Host ("code=" + $bd2.code + " status=" + $bd2.data.status)

Write-Host '===== 10. 已绑指纹再 bind 另一台(应 3003 附已绑名) ====='
$dev2 = $alloc.data.allocated[1]
$cf = Invoke-RestMethod -Method Post -Uri "$base/device/bind" -ContentType 'application/json' `
  -Body (@{merchantNo='M001';deviceName=$dev2;fingerprint=$fp;deviceInfo=@{osType='win';appVersion='1.0.0'}} | ConvertTo-Json -Depth 5)
Write-Host ("code=" + $cf.code + " msg=" + $cf.message)
Write-Host '===== 10b. 异指纹 bind 第二台(应成功) ====='
$ok2 = Invoke-RestMethod -Method Post -Uri "$base/device/bind" -ContentType 'application/json' `
  -Body (@{merchantNo='M001';deviceName=$dev2;fingerprint=$fp2;deviceInfo=@{osType='android';appVersion='1.1.3'}} | ConvertTo-Json -Depth 5)
Write-Host ("code=" + $ok2.code + " bound=" + $ok2.data.deviceName)

Write-Host '===== 11. 设备列表查平台/版本列 ====='
$dl = Invoke-RestMethod -Method Get -Uri "$base/devices?pageSize=100" -Headers $h
foreach ($n in @($dev1,$dev2)) {
  $boundDev = $dl.data.items | Where-Object { $_.name -eq $n }
  Write-Host ("dev=" + $boundDev.name + " state=" + $boundDev.state + " osType=" + $boundDev.osType + " appVersion=" + $boundDev.appVersion + " online=" + $boundDev.online)
}

Write-Host '===== 12. dashboard ====='
$dash = Invoke-RestMethod -Method Get -Uri "$base/dashboard" -Headers $h
Write-Host ("merchant=" + $dash.data.merchantCount + " inventory=" + $dash.data.inventoryCount + " allocated=" + $dash.data.allocatedCount + " bound=" + $dash.data.boundCount)
Write-Host ("loginStats today=" + ($dash.data.loginStats | Where-Object { $_.success -gt 0 -or $_.failed -gt 0 } | ConvertTo-Json -Compress))

Write-Host '===== 12b. 批量回收(1台已分配未绑+1台已绑+1台库存+1台不存在) ====='
$invName2 = ($dl.data.items | Where-Object { $_.state -eq 'inventory' } | Select-Object -First 1).name
$br = Invoke-RestMethod -Method Post -Uri "$base/devices/batch-reclaim" -Headers $h -ContentType 'application/json' `
  -Body (@{names=@($alloc2.data.allocated[0],$dev1,$invName2,'ZYDY_FAKE_0000000001')} | ConvertTo-Json -Depth 3)
Write-Host ("reclaimed=" + ($br.data.reclaimed -join ','))
foreach ($k in $br.data.skipped.PSObject.Properties) { Write-Host ("skipped: " + $k.Name + " -> " + $k.Value) }

Write-Host '===== 13. 批量解绑(2台已绑+1台未绑+1台不存在) ====='
$invName = ($dl.data.items | Where-Object { $_.state -eq 'inventory' } | Select-Object -First 1).name
$bu = Invoke-RestMethod -Method Post -Uri "$base/devices/batch-unbind" -Headers $h -ContentType 'application/json' `
  -Body (@{names=@($dev1,$dev2,$invName,'ZYDY_FAKE_0000000001')} | ConvertTo-Json -Depth 3)
Write-Host ("unbound=" + ($bu.data.unbound -join ','))
foreach ($k in $bu.data.skipped.PSObject.Properties) { Write-Host ("skipped: " + $k.Name + " -> " + $k.Value) }

Write-Host '===== 14. 重复批量解绑(应全部跳过) ====='
$bu2 = Invoke-RestMethod -Method Post -Uri "$base/devices/batch-unbind" -Headers $h -ContentType 'application/json' `
  -Body (@{names=@($dev1,$dev2)} | ConvertTo-Json -Depth 3)
Write-Host ("unbound=" + @($bu2.data.unbound).Count + " skipped=" + @($bu2.data.skipped.PSObject.Properties).Count)

Write-Host '===== 15. 删除商户1(已无绑定设备,应成功) ====='
$dm = Invoke-RestMethod -Method Delete -Uri "$base/merchants/1" -Headers $h
Write-Host ("released=" + $dm.data.released)

Write-Host '===== 16. 删除不存在的商户(应2001) ====='
$dm2 = Invoke-RestMethod -Method Delete -Uri "$base/merchants/999" -Headers $h
Write-Host ("code=" + $dm2.code + " msg=" + $dm2.message)

Write-Host '===== 17. 终态 dashboard ====='
$dash2 = Invoke-RestMethod -Method Get -Uri "$base/dashboard" -Headers $h
Write-Host ("merchant=" + $dash2.data.merchantCount + " inventory=" + $dash2.data.inventoryCount + " allocated=" + $dash2.data.allocatedCount + " bound=" + $dash2.data.boundCount)
Write-Host '===== SMOKE DONE ====='
