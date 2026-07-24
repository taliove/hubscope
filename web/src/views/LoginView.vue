<template>
  <div class="login-page">
    <div class="login-stack">
      <!-- Brand block above the card (ui-guidelines §2b): BrandMark +
           Wordmark (display step) + subtitle; the page renders no AppHeader. -->
      <div class="login-brand">
        <div class="login-brand-row">
          <BrandMark class="login-mark" />
          <Wordmark class="login-wordmark" />
        </div>
        <span class="login-subtitle">LLM Hub 监控与评估平台</span>
      </div>
      <el-card class="login-card">
        <el-form @submit.prevent="onSubmit">
          <el-form-item>
            <el-input
              v-model="username"
              placeholder="账号"
              autofocus
              @keyup.enter="onSubmit"
            />
          </el-form-item>
          <el-form-item>
            <el-input
              v-model="password"
              type="password"
              placeholder="密码"
              show-password
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
import BrandMark from '@/components/BrandMark.vue'
import Wordmark from '@/components/Wordmark.vue'

const route = useRoute()
const router = useRouter()
const username = ref('')
const password = ref('')
const submitting = ref(false)

// Submit account + password; on success bounce back to the originally
// requested page. On failure the password is cleared (keep the username
// so the user can retry without retyping it) — a common security UX.
async function onSubmit() {
  if (submitting.value) return
  if (!username.value) {
    ElMessage.error('请输入账号')
    return
  }
  if (!password.value) {
    ElMessage.error('请输入密码')
    return
  }
  submitting.value = true
  try {
    await login({ username: username.value, password: password.value })
    const redirect = typeof route.query.redirect === 'string' ? route.query.redirect : '/admin'
    router.replace(redirect)
  } catch (err) {
    ElMessage.error((err as Error).message)
    password.value = ''
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
.login-brand-row {
  display: flex;
  align-items: center;
  gap: 12px;
}
.login-mark {
  font-size: 40px;
}
.login-wordmark {
  font-size: var(--hs-text-display);
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
