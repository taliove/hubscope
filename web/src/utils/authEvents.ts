// Auth-change event channel (GH #148, 2026-08-02 user ruling): the shell has
// no state store, so session state is checked locally per component on mount
// and on route change. That left a hole — logging out while already on `/`
// changes no route, so the sidebar menu, the user card, and the RecentEvents
// gate stayed in the logged-in state. This window CustomEvent is the second
// channel: LoginView dispatches after a successful login, AppTopbar after a
// successful logout (only on success — a failed logout keeps the session
// UI), and AppSidebar / DashboardView / AlertsView listen and re-check
// (DashboardView owns the `authed` prop — RecentEvents converges through
// it rather than holding a second session source, main ruling on GH #148).
// The name is frozen here so emit and listen sides can never drift apart.
export const AUTH_CHANGED_EVENT = 'hs:auth-changed'

// dispatchAuthChanged notifies every shell listener. Called only AFTER the
// session transition has completed server-side, so a listener's re-check
// always observes the new state.
export function dispatchAuthChanged(): void {
  window.dispatchEvent(new CustomEvent(AUTH_CHANGED_EVENT))
}
