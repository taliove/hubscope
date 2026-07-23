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
    <div class="preview">
      <StatusCard
        v-if="snapshot"
        :entries="snapshot.entries"
        :keyword="snapshot.keyword"
        :protocol="snapshot.protocol"
        :status="snapshot.status"
        :generated-at="snapshot.generatedAt"
        :origin="origin"
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
      :generated-at="snapshot.generatedAt"
      :origin="origin"
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
// Preview + actions for the StatusCard (ticket 49). The exported PNG comes
// from an offscreen twin of the preview (see the capture-source comment
// below): snapdom applies ancestor overflow clipping to its output, so the
// scroll-capped preview cannot be the capture source. Failures keep the
// buttons usable — the buttons themselves are the retry path
// (ui-guidelines §6).
import { ref, computed } from 'vue'
import { ElMessage } from 'element-plus'
import StatusCard from '@/components/StatusCard.vue'
import { canCopyImage, copyImageBlob } from '@/utils/clipboard'
import { captureStatusCard, downloadStatusCard, statusCardFilename } from '@/utils/statusCardImage'
import type { StatusCardSnapshot } from '@/utils/statusCardSnapshot'

defineProps<{
  visible: boolean
  snapshot: StatusCardSnapshot | null
}>()

const emit = defineEmits<{ (e: 'update:visible', value: boolean): void }>()

const cardRef = ref<InstanceType<typeof StatusCard> | null>(null)
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

function fail(message: string, cause: unknown) {
  const reason = cause instanceof Error ? cause.message : String(cause)
  error.value = `${message}:${reason}`
}

async function onCopy() {
  const el = cardElement()
  if (!el || copying.value) return
  copying.value = true
  error.value = null
  try {
    const blob = await captureStatusCard(el)
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
  const el = cardElement()
  if (!el || downloading.value) return
  downloading.value = true
  error.value = null
  try {
    await downloadStatusCard(el, statusCardFilename(new Date()))
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
  overflow: auto;
  /* Tall cards (long abnormal lists) scroll inside the dialog body so the
     footer actions stay on screen. */
  max-height: 62vh;
}
/* Offscreen capture twin: in-document (styles/variables resolve) but
   outside every overflow constraint and out of view. */
.capture-source {
  position: fixed;
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
