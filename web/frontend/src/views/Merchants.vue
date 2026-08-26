<template>
  <div>
    <el-card>
      <template #header>
        <div style="display: flex; justify-content: space-between; align-items: center">
          <span>商户管理</span>
          <el-button type="primary" @click="openCreate">新增商户</el-button>
        </div>
      </template>
      <el-table :data="list" border>
        <el-table-column prop="merchantNoLong" label="长商户号" min-width="140" />
        <el-table-column prop="merchantNoShort" label="短商户号" min-width="110" />
        <el-table-column prop="name" label="商户名称" min-width="140" />
        <el-table-column prop="contactPhone" label="联系电话" min-width="120" />
        <el-table-column prop="address" label="地址" min-width="160" show-overflow-tooltip />
        <el-table-column prop="allocatedCount" label="名下设备" width="90" />
        <el-table-column prop="boundCount" label="已绑定" width="80" />
        <el-table-column label="操作" width="270" fixed="right">
          <template #default="{ row }">
            <el-button size="small" type="primary" @click="openAllocate(row)">分配设备</el-button>
            <el-button size="small" @click="openDevices(row)">设备</el-button>
            <el-button size="small" type="danger" @click="doDelete(row)">删除</el-button>
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

    <!-- 新增商户 -->
    <el-dialog v-model="showCreate" title="新增商户" width="480px">
      <el-form :model="m" label-width="90px">
        <el-form-item label="长商户号"><el-input v-model="m.merchantNoLong" /></el-form-item>
        <el-form-item label="短商户号"><el-input v-model="m.merchantNoShort" /></el-form-item>
        <el-form-item label="商户名称"><el-input v-model="m.name" /></el-form-item>
        <el-form-item label="联系电话"><el-input v-model="m.contactPhone" /></el-form-item>
        <el-form-item label="地址"><el-input v-model="m.address" /></el-form-item>
        <el-form-item label="备注"><el-input v-model="m.remark" type="textarea" /></el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="showCreate = false">取消</el-button>
        <el-button type="primary" :loading="busy" @click="doCreate">确定</el-button>
      </template>
    </el-dialog>

    <!-- 分配设备 -->
    <el-dialog v-model="showAlloc" title="分配设备" width="460px">
      <el-alert type="info" :closable="false" style="margin-bottom: 12px"
        :title="'当前库存 ' + inventory + ' 台,分配后由该商户独占 30 天,到期未绑定自动回库存'">
        &nbsp;
      </el-alert>
      <el-form label-width="90px">
        <el-form-item label="分配数量">
          <el-input-number v-model="allocCount" :min="1" :max="Math.max(inventory, 1)" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="showAlloc = false">关闭</el-button>
        <el-button type="primary" :loading="busy" @click="doAllocate">确认分配</el-button>
      </template>
    </el-dialog>

    <!-- 名下设备抽屉 -->
    <el-drawer v-model="showDrawer" :title="'商户设备 - ' + (curMerchant ? curMerchant.name : '')" size="620px">
      <el-table :data="devList" border>
        <el-table-column prop="name" label="设备SN" min-width="150" />
        <el-table-column label="状态" width="120">
          <template #default="{ row }">
            <el-tag :type="stateType(row.state)">{{ stateText(row.state) }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="allocatedLeftDays" label="未激活剩余天数" width="130">
          <template #default="{ row }">{{ row.state === 'allocated' ? row.allocatedLeftDays : '—' }}</template>
        </el-table-column>
        <el-table-column label="操作" width="100">
          <template #default="{ row }">
            <el-button v-if="row.state === 'allocated'" size="small" type="warning" @click="doReclaim(row)">收回</el-button>
            <span v-else>—</span>
          </template>
        </el-table-column>
      </el-table>
    </el-drawer>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import http from '../api/http'

const list = ref([])
const total = ref(0)
const page = ref(1)
const pageSize = ref(50)
const busy = ref(false)
const inventory = ref(0)

const showCreate = ref(false)
const m = ref({})

const showAlloc = ref(false)
const allocCount = ref(1)

const showDrawer = ref(false)
const curMerchant = ref(null)
const devList = ref([])

async function load() {
  const d = await http.get('/merchants', { params: { page: page.value, pageSize: pageSize.value } })
  list.value = d.items || []
  total.value = d.total || 0
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

function openCreate() {
  m.value = { merchantNoLong: '', merchantNoShort: '', name: '', contactPhone: '', address: '', remark: '' }
  showCreate.value = true
}
async function doCreate() {
  if (!m.value.merchantNoLong || !m.value.merchantNoShort || !m.value.name) {
    ElMessage.warning('长商户号/短商户号/名称必填')
    return
  }
  busy.value = true
  try {
    await http.post('/merchants', m.value)
    ElMessage.success('新增成功')
    showCreate.value = false
    load()
  } finally {
    busy.value = false
  }
}

async function openAllocate(row) {
  curMerchant.value = row
  allocCount.value = 1
  inventory.value = (await http.get('/inventory', { params: { pageSize: 1 } })).total || 0
  if (inventory.value <= 0) {
    ElMessage.warning('库存中没有可分配的设备,请先导入设备或从商户收回')
    return
  }
  showAlloc.value = true
}
async function doAllocate() {
  if (!curMerchant.value) return
  busy.value = true
  try {
    const d = await http.post(`/merchants/${curMerchant.value.id}/allocate`, { count: allocCount.value })
    ElMessage.success('分配成功:' + (d.allocated || []).join(', '))
    showAlloc.value = false
    load()
  } finally {
    busy.value = false
  }
}

async function openDevices(row) {
  curMerchant.value = row
  devList.value = (await http.get(`/merchants/${row.id}/devices`)).items || []
  showDrawer.value = true
}
async function doDelete(row) {
  let msg
  if (row.boundCount > 0) {
    msg = `商户「${row.name}」名下有 ${row.boundCount} 台已绑定设备,将拒绝删除(须先解绑)。`
  } else if (row.allocatedCount > 0) {
    msg = `商户「${row.name}」名下 ${row.allocatedCount} 台未绑定设备将回库存。`
  } else {
    msg = `商户「${row.name}」名下无设备。`
  }
  try {
    await ElMessageBox.confirm(msg + '\n确认删除?', '删除商户', { type: 'warning' })
  } catch {
    return
  }
  try {
    const d = await http.delete(`/merchants/${row.id}`)
    ElMessage.success(`已删除,${d.released || 0} 台设备回库存`)
    load()
  } catch { /* 错误提示已由 http.js 统一弹出 */ }
}
async function doReclaim(row) {
  try {
    await ElMessageBox.confirm('确认收回该设备回库存?', '提示', { type: 'warning' })
    await http.post(`/merchants/${curMerchant.value.id}/devices/${row.name}/reclaim`)
    ElMessage.success('已收回')
    devList.value = (await http.get(`/merchants/${curMerchant.value.id}/devices`)).items || []
    load()
  } catch (e) { /* 取消或错误 */ }
}

function stateType(s) { return s === 'bound' ? 'success' : s === 'allocated' ? 'warning' : 'info' }
function stateText(s) { return s === 'bound' ? '已绑定' : s === 'allocated' ? '已分配未激活' : '库存' }

onMounted(load)
</script>
