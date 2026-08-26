<template>
  <el-container style="height: 100%">
    <el-aside width="200px" style="background: #1f2d3d">
      <div class="logo">CMF打印服务器</div>
      <el-menu :default-active="route.path" router background-color="#1f2d3d" text-color="#c8d0dc" active-text-color="#ffffff">
        <el-menu-item index="/dashboard">总览</el-menu-item>
        <el-menu-item index="/merchants">商户管理</el-menu-item>
        <el-menu-item index="/devices">设备管理</el-menu-item>
        <el-menu-item v-if="userStore.user && userStore.user.role === 'admin'" index="/accounts">账号管理</el-menu-item>
        <el-menu-item index="/profile">个人中心</el-menu-item>
      </el-menu>
    </el-aside>
    <el-container>
      <el-header style="display: flex; align-items: center; justify-content: flex-end; gap: 12px; border-bottom: 1px solid #e4e7ed">
        <span>{{ userStore.user ? userStore.user.displayName || userStore.user.username : '' }}({{ userStore.user ? userStore.user.role : '' }})</span>
        <el-button size="small" @click="logout">退出</el-button>
      </el-header>
      <el-main style="background: #f5f7fa">
        <router-view />
      </el-main>
    </el-container>
  </el-container>
</template>

<script setup>
import { useRouter, useRoute } from 'vue-router'
import { useUserStore } from '../store/user'

const route = useRoute()
const router = useRouter()
const userStore = useUserStore()

function logout() {
  userStore.logout()
  router.push('/login')
}
</script>

<style scoped>
.logo {
  color: #fff;
  font-size: 16px;
  font-weight: 600;
  text-align: center;
  padding: 18px 8px;
}
</style>
