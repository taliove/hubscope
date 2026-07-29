<template>
  <el-dialog
    :model-value="visible"
    :title="shared ? '保存图片' : '分享图片'"
    width="752px"
    class="eval-share-dialog"
    destroy-on-close
    @update:model-value="emit('update:visible', $event)"
    @closed="onClosed"
  >
    <div class="preview">
      <EvalCard v-if="snapshot" :snapshot="snapshot" :origin="origin" />
    </div>

    <!-- Offscreen twin used as the capture source. The preview caps its
         height for scrolling; capturing it would clip the PNG to the visible
         slice (ancestors with overflow clip snapdom's output). This twin
         lives in the same document, so scoped styles and CSS variables
         resolve identically, but no ancestor constrains its paint box. -->
    <EvalCard v-if="snapshot" ref="cardRef" class="capture-source" :snapshot="snapshot" :origin="origin" />

    <el-alert v-if="error" :title="error" type="error" :closable="false" class="error-alert" />

    <template #footer>
      <span v-if="!copySupported" class="copy-hint">当前环境不支持复制图片,请使用下载</span>
      <el-button :disabled="!copySupported || busy" :loading="copying" @click="onCopy">
        复制图片
      </el-button>
      <el-button type="primary" :loading="downloading" :disabled="busy && !downloading" @click="onDownload">
        下载 PNG
      </el-button>
    </template>
  </el-dialog>
</template>

<script setup lang="ts">
// Preview + actions for the EvalCard (ticket 76), structurally identical to
// the StatusShareDialog (ticket 56): the exported PNG comes from an
// offscreen twin of the preview (see the capture-source comment above), the
// capture always happens in the light theme, and failures keep the buttons
// usable — the buttons themselves are the retry path (ui-guidelines §6).
// Purely client-side (props snapshot + snapdom + local download/clipboard):
// the shared report page mounts this without any session API, keeping the
// ADR 0006 share-surface invariant.
import { ref, computed } from 'vue'
import { ElMessage } from 'element-plus/es/components/message/index'
import EvalCard from '@/components/EvalCard.vue'
import { canCopyImage, copyImageBlob } from '@/utils/clipboard'
import { captureCardImage, downloadCardImage, cardFilename } from '@/utils/cardImage'
import type { EvalCardSnapshot } from '@/utils/evalCardSnapshot'

const props = withDefaults(
  defineProps<{
    visible: boolean
    snapshot: EvalCardSnapshot | null
    // Shared report page (/report/:token): the reader is the recipient, so
    // the entry copy and the dialog title read "保存图片" (ui-guidelines §5).
    shared?: boolean
  }>(),
  { shared: false },
)

const emit = defineEmits<{ (e: 'update:visible', value: boolean): void }>()

const cardRef = ref<InstanceType<typeof EvalCard> | null>(null)
const copying = ref(false)
const downloading = ref(false)
const error = ref<string | null>(null)

// Secure-context capability is static for the page lifetime; evaluate once.
const copySupported = canCopyImage()
const origin = window.location.origin

const busy = computed(() => copying.value || downloading.value)

function cardElement(): HTMLElement | null {
  return (cardRef.value?.$el as HTMLElement | undefined) ?? null
}

// Exported material always renders in the light theme (ui-guidelines §2a):
// the PNG is an outward-facing static artifact, and dark pixels must never
// bake into it. The capture twin is reparented under a plain wrapper that
// carries the light tokens for the duration of the capture.
async function withLightCapture<T>(fn: (el: HTMLElement) => Promise<T>): Promise<T> {
  const el = cardElement()
  if (!el) throw new Error('eval card not rendered')
  if (!document.documentElement.classList.contains('dark')) {
    return fn(el)
  }
  // Keep the twin in-document (scoped styles resolve) but outside the
  // html.dark cascade: a detached wrapper subtree inherits :root tokens.
  // Absolutely positioned like the twin's own class — never fixed (see
  // .capture-source for why fixed breaks the capture stage).
  const wrapper = document.createElement('div')
  wrapper.style.cssText = 'position:absolute;left:-10000px;top:0;pointer-events:none;'
  const parent = el.parentNode!
  const next = el.nextSibling
  wrapper.appendChild(el)
  document.body.appendChild(wrapper)
  try {
    return await fn(el)
  } finally {
    parent.insertBefore(el, next)
    wrapper.remove()
  }
}

function fail(message: string, cause: unknown) {
  const reason = cause instanceof Error ? cause.message : String(cause)
  error.value = `${message}:${reason}`
}

async function onCopy() {
  if (!cardElement() || copying.value) return
  copying.value = true
  error.value = null
  try {
    const blob = await withLightCapture((el) => captureCardImage(el))
    if (await copyImageBlob(blob)) {
      ElMessage.success('图片已复制到剪贴板')
    } else {
      error.value = '复制失败:浏览器拒绝了剪贴板写入,请使用下载'
    }
  } catch (e) {
    fail('生成图片失败', e)
  } finally {
    copying.value = false
  }
}

async function onDownload() {
  if (!cardElement() || downloading.value) return
  downloading.value = true
  error.value = null
  try {
    // Scope is always the batch number; the finer scope (filters, view) is
    // stated by the in-card chips, not the filename (ui-guidelines §5).
    await withLightCapture((el) =>
      downloadCardImage(el, cardFilename(new Date(), 'eval', `批次${props.snapshot?.campaignId ?? ''}`)),
    )
    ElMessage.success('已开始下载 PNG')
  } catch (e) {
    fail('生成图片失败', e)
  } finally {
    downloading.value = false
  }
}

function onClosed() {
  error.value = null
}
</script>

<style scoped>
.preview {
  display: flex;
  justify-content: center;
  /* Row-flex default is align-items: stretch, which would squash the card
     to the container's max-height (its overflow:hidden then clips the rest
     invisibly). flex-start keeps the card at natural height so tall cards
     actually scroll. */
  align-items: flex-start;
  overflow: auto;
  /* Tall cards (up to 20 stacked-bar rows) scroll inside the dialog body so
     the footer actions stay on screen. */
  max-height: 62vh;
}
/* Offscreen capture twin: in-document (styles/variables resolve) but
   outside every overflow constraint and out of view. Absolutely positioned,
   never `fixed`: snapdom re-stages fixed elements against the viewport, which
   stretches the card beyond its 720px design width (flex tracks re-expand
   while px-frozen children stay) and breaks bar/score alignment. */
.capture-source {
  position: absolute;
  left: -10000px;
  top: 0;
  pointer-events: none;
}
.error-alert {
  margin-top: 12px;
}
.copy-hint {
  font-size: var(--hs-text-xs);
  color: var(--hs-text-secondary);
  margin-right: 12px;
}
</style>
