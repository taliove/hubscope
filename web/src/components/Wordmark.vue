<template>
  <!-- Wordmark: the only logotype. "HubScope" in the system monospace
       stack (zero external font deps, single-binary friendly) with a
       brand-colored pulse dot — always on, echoing the center dot of the
       BrandMark scope glyph. Static by design: blinking/flashing is the
       failing alert's exclusive channel (W5), so the logotype exits the
       motion semantics entirely (ui-guidelines §2b). Sizing is em-based;
       the host controls scale via font-size. -->
  <span class="hs-wordmark">
    <span class="hs-wordmark__text">HubScope</span>
    <span class="hs-wordmark__dot" aria-hidden="true"></span>
  </span>
</template>

<style scoped>
.hs-wordmark {
  display: inline-flex;
  align-items: baseline;
  gap: 0.15em;
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

.hs-wordmark__dot {
  width: 0.32em;
  height: 0.32em;
  border-radius: 50%;
  background: var(--hs-brand);
  transform: translateY(-0.04em);
}
</style>

