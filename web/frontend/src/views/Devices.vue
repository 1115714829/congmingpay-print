<template>
  <div>
    <el-card>
      <template #header>
        <div style="display: flex; justify-content: space-between; align-items: center">
          <el-input v-model="keyword" placeholder="搜索 设备SN/商户号/商户名" style="width: 260px" clearable @input="load" />
          <div>
            <el-button v-if="isAdmin" :disabled="selected.length === 0" @click="doBatchUnbind">
              批量解绑{{ selected.length ? ' ' + selected.length + ' 台' : '' }}
            </el-button>
            <el-button v-if="isAdmin" :disabled="selected.length === 0" @click="doBatchReclaim">
              批量回收{{ selected.length ? ' ' + selected.length + ' 台' : '' }}
            </el-button>
            <el-button v-if="isAdmin" type="primary" @click="showImport = true">导入设备</el-button>
          </div>
        </div>
      </template>
      <el-table :data="list" border v-loading="loading" @selection-change="onSelChange">
        <el-table-column v-if="isAdmin" type="selection" width="45" />
        <el-table-column prop="name" label="设备SN(DeviceName)" min-width="160" />
        <el-table-column prop="deviceSecret" label="设备密钥(DeviceSecret)" min-width="180" show-overflow-tooltip />
        <el-table-column prop="productKey" label="通信密钥(ProductKey)" min-width="140" />
        <el-table-column label="所属商户" min-width="140">
          <template #default="{ row }">{{ row.merchantName || '—' }}</template>
        </el-table-column>
        <el-table-column label="分配时间" min-width="160">
          <template #default="{ row }">{{ fmtTime(row.allocatedAt) }}</template>
        </el-table-column>
        <el-table-column label="平台/版本" min-width="140">
          <template #default="{ row }">{{ osText(row) }}</template>
        </el-table-column>
        <el-table-column label="状态" width="120">
          <template #default="{ row }">
            <el-tag :type="stateType(row.state)">{{ stateText(row.state) }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="最近上线" min-width="160">
          <template #default="{ row }">{{ row.state === 'bound' ? fmtTime(row.lastSeenAt) : '—' }}</template>
        </el-table-column>
        <el-table-column label="在线" width="70">
          <template #default="{ row }">
            <el-tag :type="row.online ? 'success' : 'info'" size="small">{{ row.online ? '在线' : '离线' }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="操作" width="150" fixed="right">
          <template #default="{ row }">
            <el-button size="small" @click="openDetail(row)">详情</el-button>
            <el-button v-if="isAdmin" size="small" type="danger" :disabled="row.state !== 'bound'" @click="doUnbind(row)">解绑</el-button>
          </template>
        </el-table-column>
      </el-table>
      <el-pagination
        style="margin-top: 12px; justify-content: flex-end"
        layout="total, sizes, prev, pager, next"
        :page-sizes="[10, 20, 50, 100]"
        :total="total" :page-size="pageSize" :current-page="page"
        @current-change="onPage" @size-change="onSize" />
    </el-card>

    <!-- 导入设备 -->
    <el-dialog v-model="showImport" title="导入设备(CSV)" width="460px">
      <el-alert type="info" :closable="false" style="margin-bottom: 12px"
        title="模板三列:DeviceName,DeviceSecret,ProductKey(含表头);任一校验失败整批拒绝零写入">
        &nbsp;
      </el-alert>
      <el-upload drag :auto-upload="false" :limit="1" accept=".csv" :on-change="onFile" :on-exceed="onExceed">
        <el-icon style="font-size: 40px; color: #909399"><upload-filled /></el-icon>
        <div>将 CSV 文件拖到此处,或<em>点击选择</em></div>
      </el-upload>
      <div v-if="importMsg" style="margin-top: 10px" :style="{ color: importOk ? '#67c23a' : '#f56c6c' }">{{ importMsg }}</div>
      <template #footer>
        <el-button @click="showImport = false">关闭</el-button>
        <el-button type="primary" :loading="busy" @click="doImport">开始导入</el-button>
      </template>
    </el-dialog>

    <!-- 设备详情 -->
    <el-drawer v-model="showDetail" :title="'设备详情 - ' + (detail.name || '')" size="520px">
      <el-descriptions :column="1" border>
        <el-descriptions-item label="设备SN">{{ detail.name }}</el-descriptions-item>
        <el-descriptions-item label="设备密钥">{{ detail.deviceSecret }}</el-descriptions-item>
        <el-descriptions-item label="通信密钥">{{ detail.productKey }}</el-descriptions-item>
        <el-descriptions-item label="状态">{{ stateText(detail.state) }}</el-descriptions-item>
        <el-descriptions-item label="平台/版本">{{ osText(detail) }}</el-descriptions-item>
        <el-descriptions-item label="绑定时间">{{ fmtTime(detail.boundAt) }}</el-descriptions-item>
        <el-descriptions-item label="最近上线">{{ detail.state === 'bound' ? fmtTime(detail.lastSeenAt) : '—' }}</el-descriptions-item>
        <el-descriptions-item label="绑定指纹">
          <pre style="margin: 0; white-space: pre-wrap; word-break: break-all; font-size: 12px">{{ prettyFp(detail.fingerprint) }}</pre>
        </el-descriptions-item>
      </el-descriptions>
    </el-drawer>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { UploadFilled } from '@element-plus/icons-vue'
import http from '../api/http'
import { useUserStore } from '../store/user'

const userStore = useUserStore()
const isAdmin = computed(() => userStore.user && userStore.user.role === 'admin')

const list = ref([])
const total = ref(0)
const page = ref(1)
const pageSize = ref(50)
const keyword = ref('')
const loading = ref(false)

const showImport = ref(false)
const file = ref(null)
const importMsg = ref('')
const importOk = ref(false)
const busy = ref(false)

const showDetail = ref(false)
const detail = ref({})

const selected = ref([])
function onSelChange(rows) { selected.value = rows }

async function load() {
  loading.value = true
  try {
    const d = await http.get('/devices', { params: { page: page.value, pageSize: pageSize.value, keyword: keyword.value || '' } })
    list.value = d.items || []
    total.value = d.total || 0
  } finally {
    loading.value = false
  }
}

// 平台/版本列:仅已绑定设备有意义(解绑后数据已失效),其余显示 —
function osText(row) {
  if (row.state !== 'bound') return '—'
  const os = row.osType === 'win' ? 'Windows' : row.osType === 'android' ? 'Android' : ''
  if (!os) return '—'
  return os + (row.appVersion ? ' ' + row.appVersion : '')
}
function stateType(s) { return s === 'bound' ? 'success' : s === 'allocated' ? 'warning' : 'info' }
function stateText(s) { return s === 'bound' ? '已绑定' : s === 'allocated' ? '已分配未激活' : '库存' }
function fmtTime(v) {
  if (!v) return '—'
  return v.replace('T', ' ').replace('Z', '')
}
function prettyFp(v) {
  if (!v) return '(未绑定)'
  try { return JSON.stringify(JSON.parse(v), null, 2) } catch { return v }
}

function onPage(p) {
  page.value = p
  load()
}
function onSize(s) {
  pageSize.value = s
  page.value = 1
  load()
}

function onFile(f) { file.value = f.raw }
function onExceed() { ElMessage.warning('一次只能导入一个文件') }
async function doImport() {
  if (!file.value) {
    ElMessage.warning('请选择 CSV 文件')
    return
  }
  busy.value = true
  importMsg.value = ''
  try {
    const fd = new FormData()
    fd.append('file', file.value)
    const d = await http.post('/inventory/import', fd)
    importOk.value = true
    importMsg.value = `导入成功:${d.imported}/${d.total} 台`
    ElMessage.success('导入成功')
    file.value = null
    load()
  } catch (e) {
    importOk.value = false
    importMsg.value = e.message || '导入失败'
  } finally {
    busy.value = false
  }
}

async function openDetail(row) {
  detail.value = await http.get(`/devices/${row.name}`)
  showDetail.value = true
}
async function doUnbind(row) {
  try {
    await ElMessageBox.confirm(`确认解绑 ${row.name}?设备将回到库存,指纹绑定清除。`, '提示', { type: 'warning' })
  } catch { return }
  try {
    await http.post(`/devices/${row.name}/unbind`)
    ElMessage.success('已解绑')
    if (showDetail.value && detail.value.name === row.name) {
      detail.value = await http.get(`/devices/${row.name}`)
    }
    load()
  } catch { /* 错误提示已由 http.js 统一弹出 */ }
}
async function doBatchUnbind() {
  if (!selected.value.length) return
  const names = selected.value.map(r => r.name)
  try {
    await ElMessageBox.confirm(`确认解绑所选 ${names.length} 台设备?仅已绑定台生效,其余跳过。`, '批量解绑', { type: 'warning' })
  } catch { return }
  try {
    const d = await http.post('/devices/batch-unbind', { names })
    const skip = Object.keys(d.skipped || {}).length
    ElMessage.success(`已解绑 ${d.unbound.length} 台` + (skip ? `,跳过 ${skip} 台` : ''))
    load()
  } catch { /* 错误提示已由 http.js 统一弹出 */ }
}
async function doBatchReclaim() {
  if (!selected.value.length) return
  const names = selected.value.map(r => r.name)
  try {
    await ElMessageBox.confirm(`确认回收所选 ${names.length} 台设备回库存?仅已分配未激活台生效,已绑定/已在库存的跳过。`, '批量回收', { type: 'warning' })
  } catch { return }
  try {
    const d = await http.post('/devices/batch-reclaim', { names })
    const skip = Object.keys(d.skipped || {}).length
    ElMessage.success(`已回收 ${d.reclaimed.length} 台` + (skip ? `,跳过 ${skip} 台` : ''))
    load()
  } catch { /* 错误提示已由 http.js 统一弹出 */ }
}

onMounted(load)
</script>
