import { createRouter, createWebHistory } from 'vue-router'

const routes = [
  { path: '/login', name: 'login', component: () => import('../views/Login.vue'), meta: { public: true } },
  {
    path: '/',
    component: () => import('../layout/MainLayout.vue'),
    redirect: '/dashboard',
    children: [
      { path: 'dashboard', name: 'dashboard', component: () => import('../views/Dashboard.vue'), meta: { title: '总览' } },
      { path: 'merchants', name: 'merchants', component: () => import('../views/Merchants.vue'), meta: { title: '商户管理' } },
      { path: 'devices', name: 'devices', component: () => import('../views/Devices.vue'), meta: { title: '设备管理' } },
      { path: 'accounts', name: 'accounts', component: () => import('../views/Accounts.vue'), meta: { title: '账号管理', admin: true } },
      { path: 'profile', name: 'profile', component: () => import('../views/Profile.vue'), meta: { title: '个人中心' } }
    ]
  },
  { path: '/:pathMatch(.*)*', redirect: '/dashboard' }
]

const router = createRouter({ history: createWebHistory(), routes })

router.beforeEach((to) => {
  const user = JSON.parse(sessionStorage.getItem('user') || 'null')
  if (!to.meta.public && !user) return '/login'
  if (to.meta.admin && user && user.role !== 'admin') return '/dashboard'
  return true
})

export default router
