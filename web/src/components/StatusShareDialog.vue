<template>
  <el-dialog
    :model-value="visible"
    title="分享状态"
    width="752px"
    class="status-share-dialog"
    destroy-on-close
    @update:model-value="emit('update:visible', $event)"
    @closed="onClosed"
  >
    <!-- Variant selector (GH #93): full vs compact toggle. -->
    <el-radio-group v-model="variant" size="small" class="variant-selector">
      <el-radio-button label="full">完整版</el-radio-button>
      <el-radio-button label="compact">紧凑版</el-radio-button>
    </el-radio-group>

    <div class="preview">
      <StatusCard
        v-if="snapshot"
        :entries="snapshot.entries"
        :keyword="snapshot.keyword"
        :protocol="snapshot.protocol"
        :status="snapshot.status"
        :group="snapshot.group"
        :generated-at="snapshot.generatedAt"
        :origin="origin"
        :hub-name="snapshot.hubName"
        :eval-summary="snapshot.evalSummary"
        :compact="variant === 'compact'"
      />
    </div>

    <!-- Offscreen twin used as the capture source. The preview caps its
         height for scrolling; capturing it would clip the PNG to the visible
         slice (ancestors with overflow clip snapdom's output). This twin
         lives in the same document, so scoped styles and CSS variables
         resolve identically, but no ancestor constrains its paint box. -->
    <StatusCard
      v-if="snapshot"
      ref="cardRef"
      class="capture-source"
      :entries="snapshot.entries"
      :keyword="snapshot.keyword"
      :protocol="snapshot.protocol"
      :status="snapshot.status"
      :group="snapshot.group"
      :generated-at="snapshot.generatedAt"
      :origin="origin"
      :hub-name="snapshot.hubName"
      :eval-summary="snapshot.evalSummary"
      :compact="variant === 'compact'"
    />

    <el-alert
      v-if="error"
      :title="error"
      type="error"
      :closable="false"
      class="error-alert"
    />

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
// Preview + actions for the StatusCard (ticket 56). The exported PNG comes
// from an offscreen twin of the preview (see the capture-source comment
// below): snapdom applies ancestor overflow clipping to its output, so the
// scroll-capped preview cannot be the capture source. Failures keep the
// buttons usable — the buttons themselves are the retry path
// (ui-guidelines §6). GH #93: variant toggle (full/compact) defaults based on
// viewport width at open time (<768px → compact).
import { ref, computed, watch } from 'vue'
import { ElMessage } from 'element-plus/es/components/message/index'
import StatusCard from '@/components/StatusCard.vue'
import { canCopyImage, copyImageBlob } from '@/utils/clipboard'
import { captureCardImage, downloadCardImage, cardFilename, type CardVariant } from '@/utils/cardImage'
import type { StatusCardSnapshot } from '@/utils/statusCardSnapshot'

const props = defineProps<{
  visible: boolean
  snapshot: StatusCardSnapshot | null
}>()

const emit = defineEmits<{ (e: 'update:visible', value: boolean): void }>()

const cardRef = ref<InstanceType<typeof StatusCard> | null>(null)
const copying = ref(false)
const downloading = ref(false)
const error = ref<string | null>(null)

// Variant state (GH #93): defaults based on viewport width at dialog open.
// One-time matchMedia check when visible becomes true (onBannerInspect mode);
// no resize listener, no persistence (share intent varies per occasion).
const variant = ref<CardVariant>('full')

// Watch visible prop to set default variant on open
watch(
  () => props.visible,
  (nowVisible) => {
    if (nowVisible) {
      const isNarrowViewport = window.matchMedia('(max-width: 767px)').matches
      variant.value = isNarrowViewport ? 'compact' : 'full'
    }
  },
)

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
  if (!el) throw new Error('status card not rendered')
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
    await withLightCapture((el) =>
      downloadCardImage(el, cardFilename(new Date(), 'status', props.snapshot?.group?.key, variant.value)),
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
.variant-selector {
  margin-bottom: 16px;
}
.preview {
  display: flex;
  justify-content: center;
  /* Row-flex default is align-items: stretch, which would squash the card
     to the container's max-height (its overflow:hidden then clips the rest
     invisibly). flex-start keeps the card at natural height so tall cards
     actually scroll. */
  align-items: flex-start;
  overflow: auto;
  /* Tall cards (long abnormal lists) scroll inside the dialog body so the
     footer actions stay on screen. */
  max-height: 62vh;
}
/* Offscreen capture twin: in-document (styles/variables resolve) but
   outside every overflow constraint and out of view. Absolutely positioned,
   never `fixed`: snapdom re-stages fixed elements against the viewport, which
   stretches the card beyond its 720px design width (flex tracks re-expand
   while px-frozen children stay) and breaks column alignment. */
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
