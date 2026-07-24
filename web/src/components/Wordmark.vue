<template>
  <!-- Wordmark: the only logotype. "HubScope" in the system monospace
       stack (zero external font deps, single-binary friendly) with a
       brand-colored terminal cursor block. Cursor blink is a registered
       exception to "blink is failing-only": it is brand furniture, not
       status semantics (ui-guidelines §2b). Sizing is em-based; the host
       controls scale via font-size. -->
  <span class="hs-wordmark">
    <span class="hs-wordmark__text">HubScope</span>
    <span class="hs-wordmark__cursor" aria-hidden="true"></span>
  </span>
</template>

<style scoped>
.hs-wordmark {
  display: inline-flex;
  align-items: baseline;
  gap: 0.12em;
  font-family:
    ui-monospace,
    'SF Mono',
    'Cascadia Mono',
    Consolas,
    monospace;
  font-weight: 700;
  font-size: 1em;
  line-height: 1;
  color: var(--hs-text-primary);
  letter-spacing: -0.02em;
}

.hs-wordmark__cursor {
  width: 0.55em;
  height: 1em;
  background: var(--hs-brand);
  border-radius: 1px;
  transform: translateY(0.12em);
  animation: hs-wordmark-cursor 1.2s step-end infinite;
}

@keyframes hs-wordmark-cursor {
  0%,
  100% {
    opacity: 1;
  }
  50% {
    opacity: 0;
  }
}

@media (prefers-reduced-motion: reduce) {
  .hs-wordmark__cursor {
    animation: none;
  }
}
</style>
