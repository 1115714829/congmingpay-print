<template>
  <div>
    <el-card>
      <template #header>
        <div style="display: flex; justify-content: space-between; align-items: center">
          <el-input v-model="keyword" placeholder="搜索 用户名/显示名" style="width: 240px" clearable @input="load" />
          <el-button type="primary" @click="openCreate">新建账号</el-button>
        </div>
      </template>
      <el-table :data="list" border v-loading="loading">
        <el-table-column prop="username" label="用户名" min-width="120" />
        <el-table-column prop="displayName" label="显示名" min-width="120" />
        <el-table-column label="角色" width="100">
          <template #default="{ row }">
            <el-tag :type="row.role === 'admin' ? 'danger' : 'info'">{{ row.role === 'admin' ? '管理员' : '操作员' }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="状态" width="90">
          <template #default="{ row }">
            <el-tag :type="row.enabled ? 'success' : 'info'">{{ row.enabled ? '启用' : '停用' }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="创建时间" min-width="160">
          <template #default="{ row }">{{ fmtTime(row.createdAt) }}</template>
        </el-table-column>
        <el-table-column label="操作" width="260" fixed="right">
          <template #default="{ row }">
            <el-button size="small" @click="openEdit(row)">编辑</el-button>
            <el-button size="small" type="warning" @click="openReset(row)">重置密码</el-button>
            <el-button size="small" :type="row.enabled ? 'danger' : 'success'" @click="doToggle(row)">
              {{ row.enabled ? '停用' : '启用' }}
            </el-button>
          </template>
        </el-table-column>
      </el-table>
      <el-pagination
        style="margin-top: 12px; justify-content: flex-end"
        layout="total, prev, pager, next"
        :total="total" :page-size="pageSize" :current-page="page"
        @current-change="onPage" />
    </el-card>

    <!-- 新建/编辑 -->
    <el-dialog v-model="showEdit" :title="editing ? '编辑账号' : '新建账号'" width="440px">
      <el-form :model="form" label-width="80px">
        <el-form-item label="用户名">
          <el-input v-model="form.username" :disabled="editing" />
        </el-form-item>
        <el-form-item v-if="!editing" label="密码">
          <el-input v-model="form.password" type="password" show-password placeholder="6-64 位,含字母和数字" />
        </el-form-item>
        <el-form-item label="显示名"><el-input v-model="form.displayName" /></el-form-item>
        <el-form-item label="角色">
          <el-select v-model="form.role" style="width: 100%">
            <el-option label="操作员" value="operator" />
            <el-option label="管理员" value="admin" />
          </el-select>
        </el-form-item>
        <el-form-item v-if="editing" label="状态">
          <el-switch v-model="form.enabled" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="showEdit = false">取消</el-button>
        <el-button type="primary" :loading="busy" @click="doSave">确定</el-button>
      </template>
    </el-dialog>

    <!-- 重置密码 -->
    <el-dialog v-model="showReset" title="重置密码" width="400px">
      <el-form label-width="80px">
        <el-form-item label="新密码">
          <el-input v-model="newPass" type="password" show-password placeholder="6-64 位,含字母和数字" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="showReset = false">取消</el-button>
        <el-button type="primary" :loading="busy" @click="doReset">确定</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'
import { ElMessage } from 'element-plus'
import http from '../api/http'

const list = ref([])
const total = ref(0)
const page = ref(1)
const pageSize = ref(20)
const keyword = ref('')
const loading = ref(false)
const busy = ref(false)

const showEdit = ref(false)
const editing = ref(false)
const form = ref({})

const showReset = ref(false)
const resetId = ref(0)
const newPass = ref('')

async function load() {
  loading.value = true
  try {
    const d = await http.get('/accounts', { params: { page: page.value, pageSize: pageSize.value, keyword: keyword.value || '' } })
    list.value = d.items || []
    total.value = d.total || 0
  } finally {
    loading.value = false
  }
}
function onPage(p) {
  page.value = p
  load()
}
function fmtTime(v) {
  return v ? v.replace('T', ' ').replace('Z', '') : '—'
}

function openCreate() {
  editing.value = false
  form.value = { username: '', password: '', displayName: '', role: 'operator' }
  showEdit.value = true
}
function openEdit(row) {
  editing.value = true
  form.value = { id: row.id, displayName: row.displayName, role: row.role, enabled: row.enabled }
  showEdit.value = true
}
async function doSave() {
  busy.value = true
  try {
    if (editing.value) {
      await http.put(`/accounts/${form.value.id}`, {
        displayName: form.value.displayName,
        role: form.value.role,
        enabled: form.value.enabled
      })
      ElMessage.success('已保存')
    } else {
      await http.post('/accounts', form.value)
      ElMessage.success('已创建')
    }
    showEdit.value = false
    load()
  } finally {
    busy.value = false
  }
}

function openReset(row) {
  resetId.value = row.id
  newPass.value = ''
  showReset.value = true
}
async function doReset() {
  busy.value = true
  try {
    await http.put(`/accounts/${resetId.value}/reset-password`, { newPassword: newPass.value })
    ElMessage.success('密码已重置')
    showReset.value = false
  } finally {
    busy.value = false
  }
}
async function doToggle(row) {
  await http.put(`/accounts/${row.id}`, { enabled: !row.enabled })
  ElMessage.success(row.enabled ? '已停用' : '已启用')
  load()
}

onMounted(load)
</script>
