<template>
  <!-- PublicFooter is the single quiet admin entry of every public page
       (ticket 90, ui-guidelines §5): anonymous visitors get no login
       button in the header — the entry retreats to this hairline footer
       line, identical on the status board, endpoint detail and /board.
       Rendered regardless of session state (same as the /board precedent);
       /login itself carries no footer. Footer composition: copyright left,
       admin entry right (2026-07-28 design decision) so the quiet entry
       does not float alone. -->
  <footer class="public-footer">
    <span class="copyright">© 2026 HubScope</span>
    <router-link to="/login" class="admin-link">管理登录</router-link>
  </footer>
</template>

<style scoped>
.public-footer {
  margin-top: var(--hs-space-5);
  padding-top: var(--hs-space-4);
  border-top: 1px solid var(--hs-border-light);
  display: flex;
  align-items: center;
  justify-content: space-between;
}
.copyright {
  font-size: var(--hs-text-xs);
  color: var(--hs-text-placeholder);
}
.admin-link {
  font-size: var(--hs-text-xs);
  color: var(--hs-text-placeholder);
  text-decoration: none;
}
.admin-link:hover {
  color: var(--hs-brand-hover);
}
</style>
