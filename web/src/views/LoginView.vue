<template>
  <div class="login-page">
    <div class="login-stack">
      <!-- Brand block above the card (spec §4.2): 40px logo tile, wordmark
           and platform subtitle; the page renders no AppHeader. -->
      <div class="login-brand">
        <span class="login-logo">HS</span>
        <span class="login-wordmark">HubScope</span>
        <span class="login-subtitle">LLM Hub 监控与评估平台</span>
      </div>
      <el-card class="login-card">
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
  background: var(--hs-bg-page);
}
.login-stack {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 24px;
}
.login-brand {
  display: flex;
  flex-direction: column;
  align-items: center;
  gap: 8px;
}
.login-logo {
  width: 40px;
  height: 40px;
  border-radius: var(--hs-radius);
  background: var(--hs-brand);
  color: var(--hs-bg-card);
  font-size: var(--hs-text-md);
  font-weight: 600;
  display: flex;
  align-items: center;
  justify-content: center;
}
.login-wordmark {
  font-size: var(--hs-text-xl);
  font-weight: 600;
  color: var(--hs-text-primary);
}
.login-subtitle {
  font-size: var(--hs-text-sm);
  color: var(--hs-text-secondary);
}
.login-card {
  width: 360px;
  max-width: calc(100vw - 32px);
}
.login-button {
  width: 100%;
}
</style>
