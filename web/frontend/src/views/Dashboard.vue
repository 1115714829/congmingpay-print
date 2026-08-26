<template>
  <div>
    <el-row :gutter="16">
      <el-col :span="6" v-for="c in cards" :key="c.label">
        <el-card>
          <div class="stat-num" :style="{ color: c.color }">{{ c.value }}</div>
          <div class="stat-label">{{ c.label }}</div>
        </el-card>
      </el-col>
    </el-row>
    <el-card style="margin-top: 16px">
      <template #header>近 7 天登录统计</template>
      <el-table :data="loginStats" border>
        <el-table-column prop="date" label="日期" width="140" />
        <el-table-column prop="success" label="成功" />
        <el-table-column prop="failed" label="失败" />
      </el-table>
    </el-card>
  </div>
</template>

<script setup>
import { ref, computed, onMounted } from 'vue'
import http from '../api/http'

const data = ref({ merchantCount: 0, inventoryCount: 0, allocatedCount: 0, boundCount: 0, loginStats: [] })

const cards = computed(() => [
  { label: '商户数', value: data.value.merchantCount, color: '#409eff' },
  { label: '库存设备', value: data.value.inventoryCount, color: '#67c23a' },
  { label: '已分配未激活', value: data.value.allocatedCount, color: '#e6a23c' },
  { label: '已绑定', value: data.value.boundCount, color: '#f56c6c' }
])
const loginStats = computed(() => data.value.loginStats || [])

onMounted(async () => {
  data.value = await http.get('/dashboard')
})
</script>

<style scoped>
.stat-num { font-size: 32px; font-weight: 700; }
.stat-label { color: #909399; margin-top: 6px; }
</style>
