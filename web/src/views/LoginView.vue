<template>
  <div class="login-page">
    <el-card class="login-card">
      <template #header>
        <div class="login-title">HubScope 管理登录</div>
      </template>
      <el-form @submit.prevent="onSubmit">
        <el-form-item>
          <el-input
            v-model="password"
            type="password"
            placeholder="管理员口令"
            show-password
            autofocus
            @keyup.enter="onSubmit"
          />
        </el-form-item>
        <el-form-item>
          <el-button class="login-button" type="primary" :loading="submitting" @click="onSubmit">
            登录
          </el-button>
        </el-form-item>
      </el-form>
    </el-card>
  </div>
</template>

<script setup lang="ts">
import { ref } from 'vue'
import { useRoute, useRouter } from 'vue-router'
import { ElMessage } from 'element-plus'
import { login } from '@/api/auth'

const route = useRoute()
const router = useRouter()
const password = ref('')
const submitting = ref(false)

// Submit the password; on success bounce back to the originally requested page.
async function onSubmit() {
  if (submitting.value) return
  if (!password.value) {
    ElMessage.error('请输入管理员口令')
    return
  }
  submitting.value = true
  try {
    await login(password.value)
    const redirect = typeof route.query.redirect === 'string' ? route.query.redirect : '/admin'
    router.replace(redirect)
  } catch (err) {
    ElMessage.error((err as Error).message)
  } finally {
    submitting.value = false
  }
}
</script>

<style scoped>
.login-page {
  min-height: 100vh;
  display: flex;
  align-items: center;
  justify-content: center;
}
.login-card {
  width: 360px;
}
.login-title {
  font-size: 16px;
  font-weight: 600;
  color: #303133;
}
.login-button {
  width: 100%;
}
</style>
