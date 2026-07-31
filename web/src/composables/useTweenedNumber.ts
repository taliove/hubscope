// Vue wrapper around utils/numberTween (GH #115, spec 0018 动效体系): a
// polled scalar (10s overview cadence) glides to its new value instead of
// jumping. Tweening is display-only — the source ref stays the single
// source of truth.
//
// Semantics:
//   - null target → display null immediately (no fake sweep; 暂无数据
//     must never tween through numbers).
//   - null → value (first load, data recovery) → set immediately; a sweep
//     from zero would invent a trend the data does not have.
//   - value → value → tween over the shared 500–800ms band; a new tween
//     cancels the in-flight one (mid-flight value becomes the new start).
//   - reduced-motion → terminal value instantly (numberTween gate).
import { ref, watch, onBeforeUnmount, type Ref } from 'vue'
import { tweenNumber } from '@/utils/numberTween'

export function useTweenedNumber(target: Ref<number | null>, durationMs?: number): Ref<number | null> {
  const display = ref<number | null>(target.value) as Ref<number | null>
  let cancel: (() => void) | null = null

  watch(target, (next, prev) => {
    cancel?.()
    cancel = null
    if (next === null || prev === null || prev === undefined) {
      display.value = next
      return
    }
    cancel = tweenNumber(prev, next, v => {
      display.value = v
    }, { durationMs })
  })

  onBeforeUnmount(() => cancel?.())
  return display
}
