<template>
  <div class="login-page">
    <div class="login-stack">
      <!-- Brand block above the card (ui-guidelines §2b): BrandMark +
           Wordmark (display step) + subtitle; the page renders outside the
           sidebar shell (route meta.bare, GH #112). -->
      <div class="login-brand">
        <div class="login-brand-row">
          <BrandMark class="login-mark" />
          <Wordmark class="login-wordmark" />
        </div>
        <span class="login-subtitle">LLM Hub 监控与评估平台</span>
      </div>
      <el-card class="login-card" shadow="never">
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
          <!-- Captcha section (ticket 89, spec 0012 decision 6): hidden by
               default with zero footprint; unfolds with a smooth height
               transition when a 401 carries the frozen captcha_required
               marker (ui-guidelines §6 自适应展开区约定). -->
          <div class="captcha-expand" :class="{ 'is-expanded': captchaExpanded }">
            <div class="captcha-expand-inner">
              <el-form-item v-if="captchaExpanded" :error="captchaFieldError">
                <div class="captcha-block">
                  <div class="captcha-row">
                    <div
                      class="captcha-image"
                      :class="{ 'is-clickable': !captchaLoading && !submitting }"
                      @click="onCaptchaClick"
                    >
                      <img
                        v-if="captchaImage && !captchaLoading && !captchaError"
                        :src="captchaImage"
                        alt="验证码"
                        draggable="false"
                      />
                      <span v-else-if="captchaError" class="captcha-image-note is-error">
                        {{ captchaError }}
                      </span>
                      <span v-else class="captcha-image-note">加载中…</span>
                    </div>
                    <el-input
                      v-model="captchaAnswer"
                      class="captcha-input"
                      placeholder="验证码"
                      @input="captchaFieldError = ''"
                      @keyup.enter="onSubmit"
                    />
                  </div>
                  <div class="captcha-hint">看不清?点击图形刷新</div>
                </div>
              </el-form-item>
            </div>
          </div>
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
import { ElMessage } from 'element-plus/es/components/message/index'
import { ApiError } from '@/api/client'
import { fetchCaptcha, login } from '@/api/auth'
import BrandMark from '@/components/BrandMark.vue'
import Wordmark from '@/components/Wordmark.vue'

const route = useRoute()
const router = useRouter()
const username = ref('')
const password = ref('')
const submitting = ref(false)

// Captcha state (spec 0012): the section stays hidden until a 401 carries
// the frozen captcha_required marker, then stays expanded until login
// succeeds. Every login submit destroys the used captcha_id server-side
// (one-shot), so any 401 while expanded triggers a fresh challenge; 429
// does not (rate limiting runs before captcha verify, so the id is not
// consumed).
const captchaExpanded = ref(false)
const captchaId = ref('')
const captchaImage = ref('')
const captchaAnswer = ref('')
const captchaLoading = ref(false)
const captchaError = ref('')
const captchaFieldError = ref('')

// mapAuthErrorMessage renders contract-stable non-Chinese server messages
// in Chinese (ui-guidelines §7): credential failures keep the unified
// anti-enumeration wording, 429 becomes a throttle hint. Pure frontend
// mapping — the API contract is untouched (ticket 89 decision points).
function mapAuthErrorMessage(err: unknown): string {
  if (err instanceof ApiError) {
    if (err.status === 429) return '请求过于频繁,请稍后重试'
    if (err.status === 401 && !err.captchaRequired) return '账号或密码错误'
  }
  return err instanceof Error ? err.message : String(err)
}

// refreshCaptcha fetches a new one-shot challenge. In-flight calls are
// ignored (the issue endpoint is rate-limited at 20/min/IP, spec 0012
// decision 4 — double-clicks must not self-inflict a 429). A failed fetch
// keeps the section expanded and shows the reason inside the fixed-size
// image container; collapsing would silently waive the captcha
// requirement, which contradicts the fail-closed semantics.
async function refreshCaptcha() {
  if (captchaLoading.value) return
  captchaLoading.value = true
  captchaError.value = ''
  try {
    const challenge = await fetchCaptcha()
    captchaId.value = challenge.captcha_id
    captchaImage.value = challenge.image
    captchaAnswer.value = ''
  } catch (err) {
    captchaId.value = ''
    captchaImage.value = ''
    captchaError.value = mapAuthErrorMessage(err)
  } finally {
    captchaLoading.value = false
  }
}

// Clicking the image is the single refresh/retry path. Blocked while a
// login submit is in flight: the submit destroys the id being sent, so a
// refresh racing it would be overwritten by the post-submit refetch.
function onCaptchaClick() {
  if (captchaLoading.value || submitting.value) return
  void refreshCaptcha()
}

function resetCaptcha() {
  captchaExpanded.value = false
  captchaId.value = ''
  captchaImage.value = ''
  captchaAnswer.value = ''
  captchaError.value = ''
  captchaFieldError.value = ''
}

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
  // Client-side guard: submitting an empty answer would burn the one-shot
  // id on a verify attempt that cannot succeed. Reuses the frozen
  // contract wording — no new copy.
  if (captchaExpanded.value && captchaId.value && !captchaAnswer.value) {
    captchaFieldError.value = '请完成验证码'
    return
  }
  submitting.value = true
  try {
    await login({
      username: username.value,
      password: password.value,
      ...(captchaExpanded.value
        ? { captcha_id: captchaId.value, captcha_answer: captchaAnswer.value }
        : {}),
    })
    resetCaptcha()
    // Default landing after a direct login (no redirect query): the system
    // settings console (GH #119 — was /admin before AdminView retired).
    const redirect = typeof route.query.redirect === 'string' ? route.query.redirect : '/settings'
    router.replace(redirect)
  } catch (err) {
    const apiErr = err instanceof ApiError ? err : null
    if (apiErr?.status === 401 && apiErr.captchaRequired) {
      // Frozen contract: the marker exists only on the two captcha 401s.
      // Its presence is the unfold signal; the message (请完成验证码 /
      // 验证码错误或已过期) is shown inline per §5, never as a toast.
      captchaExpanded.value = true
      captchaFieldError.value = apiErr.message
      void refreshCaptcha()
    } else {
      if (captchaExpanded.value && apiErr?.status === 401) {
        // Captcha verify runs before the password check (spec 0012
        // decision 3), so this credential 401 still consumed the id —
        // refetch or the next submit would fail with 请完成验证码.
        captchaFieldError.value = ''
        void refreshCaptcha()
      }
      ElMessage.error(mapAuthErrorMessage(err))
    }
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
/* Captcha section (ui-guidelines §6 自适应展开区约定): smooth height
   transition via grid 0fr→1fr + opacity, zero footprint when hidden — no
   reserved placeholder in the default layout. Collapse transition is
   omitted because a successful login immediately navigates away. */
.captcha-expand {
  display: grid;
  grid-template-rows: 0fr;
  opacity: 0;
  transition:
    grid-template-rows var(--hs-transition),
    opacity var(--hs-transition);
}
.captcha-expand.is-expanded {
  grid-template-rows: 1fr;
  opacity: 1;
}
.captcha-expand-inner {
  min-height: 0;
  overflow: hidden;
}
.captcha-block {
  width: 100%;
}
/* Same-row composite layout (image + input in one row) keeps the unfold
   amplitude minimal — every pixel of expanded height shifts the centered
   login stack by half a pixel (ticket 89 design review). */
.captcha-row {
  display: flex;
  gap: var(--hs-space-2);
}
/* 108×32 is the backend contract size (internal/server/captcha.go
   NewDriverDigit(32, 108, …)); the px literals are exempt per §6 位图物料
   渲染纪律. 32px matches the EP default input height so the captcha row
   aligns with the username/password rows (2026-07-28 user feedback: the
   original 160×48 row stood taller than the rest of the form). The
   container holds this size across loading/error states to prevent layout
   shift. The bitmap renders as-is (no token mapping, no dark-theme
   adaptation, no CSS filters); the border makes it read as a bounded
   image. */
.captcha-image {
  width: 108px;
  height: 32px;
  flex: none;
  display: flex;
  align-items: center;
  justify-content: center;
  border: 1px solid var(--hs-border);
  border-radius: var(--hs-radius-sm);
  overflow: hidden;
}
.captcha-image.is-clickable {
  cursor: pointer;
}
.captcha-image img {
  display: block;
  width: 108px;
  height: 32px;
}
.captcha-image-note {
  padding: 0 var(--hs-space-1);
  font-size: var(--hs-text-xs);
  line-height: 1.3;
  color: var(--hs-text-placeholder);
  text-align: center;
}
.captcha-image-note.is-error {
  color: var(--hs-danger);
}
.captcha-input {
  flex: 1;
  min-width: 0;
}
.captcha-hint {
  margin-top: var(--hs-space-1);
  font-size: var(--hs-text-xs);
  color: var(--hs-text-secondary);
}
.login-button {
  width: 100%;
}
</style>
