import axios from 'axios'
import { ElMessage } from 'element-plus'
import router from '../router'

const http = axios.create({ baseURL: '/api/v1', timeout: 30000 })

http.interceptors.request.use((cfg) => {
  const tok = sessionStorage.getItem('token')
  if (tok) cfg.headers.Authorization = 'Bearer ' + tok
  return cfg
})

// 统一信封处理:code!=0 报错;401 跳登录
http.interceptors.response.use(
  (res) => {
    const body = res.data
    if (body && typeof body.code === 'number' && body.code !== 0) {
      ElMessage.error(body.message || '请求失败')
      return Promise.reject(new Error(body.message))
    }
    return body ? body.data : null
  },
  (err) => {
    if (err.response && err.response.status === 401) {
      sessionStorage.removeItem('token')
      sessionStorage.removeItem('user')
      router.push('/login')
      ElMessage.error('未登录或令牌过期')
    } else if (err.response && err.response.data && err.response.data.message) {
      ElMessage.error(err.response.data.message)
    } else {
      ElMessage.error(err.message || '网络错误')
    }
    return Promise.reject(err)
  }
)

export default http
