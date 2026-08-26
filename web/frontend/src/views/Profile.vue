<template>
  <div>
    <el-card style="max-width: 480px">
      <template #header>个人中心</template>
      <el-descriptions :column="1" border style="margin-bottom: 16px">
        <el-descriptions-item label="用户名">{{ userStore.user ? userStore.user.username : '' }}</el-descriptions-item>
        <el-descriptions-item label="显示名">{{ userStore.user ? userStore.user.displayName : '' }}</el-descriptions-item>
        <el-descriptions-item label="角色">{{ userStore.user ? (userStore.user.role === 'admin' ? '管理员' : '操作员') : '' }}</el-descriptions-item>
      </el-descriptions>
      <el-divider>修改密码</el-divider>
      <el-form :model="form" label-width="90px">
        <el-form-item label="原密码">
          <el-input v-model="form.oldPassword" type="password" show-password />
        </el-form-item>
        <el-form-item label="新密码">
          <el-input v-model="form.newPassword" type="password" show-password placeholder="6-64 位,含字母和数字" />
        </el-form-item>
        <el-form-item label="确认密码">
          <el-input v-model="form.confirm" type="password" show-password />
        </el-form-item>
        <el-form-item>
          <el-button type="primary" :loading="busy" @click="doSave">保存</el-button>
        </el-form-item>
      </el-form>
    </el-card>
  </div>
</template>

<script setup>
import { reactive, ref } from 'vue'
import { ElMessage } from 'element-plus'
import http from '../api/http'
import { useUserStore } from '../store/user'

const userStore = useUserStore()
const busy = ref(false)
const form = reactive({ oldPassword: '', newPassword: '', confirm: '' })

async function doSave() {
  if (form.newPassword !== form.confirm) {
    ElMessage.warning('两次输入的新密码不一致')
    return
  }
  busy.value = true
  try {
    await http.put('/me/password', { oldPassword: form.oldPassword, newPassword: form.newPassword })
    ElMessage.success('密码已修改,请重新登录')
    userStore.logout()
    window.location.href = '/login'
  } finally {
    busy.value = false
  }
}
</script>
