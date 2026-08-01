import { createApp } from 'vue'
// On-demand Element Plus (audit 2026-07-29: the full import produced a
// 1007kB vendor chunk). Register only what web/src actually uses — the
// inventory is grep-verified (`<el-*` tags, v-loading, ElMessage/ElMessageBox
// services); when adding a new EP component, register it AND its style here.
import { ElAlert } from 'element-plus/es/components/alert/index'
import { ElButton } from 'element-plus/es/components/button/index'
import { ElCard } from 'element-plus/es/components/card/index'
import { ElCheckbox } from 'element-plus/es/components/checkbox/index'
import { ElCollapse } from 'element-plus/es/components/collapse/index'
import { ElCollapseItem } from 'element-plus/es/components/collapse/index'
import { ElDialog } from 'element-plus/es/components/dialog/index'
import { ElDivider } from 'element-plus/es/components/divider/index'
import { ElDrawer } from 'element-plus/es/components/drawer/index'
import { ElDropdown } from 'element-plus/es/components/dropdown/index'
import { ElDropdownItem } from 'element-plus/es/components/dropdown/index'
import { ElDropdownMenu } from 'element-plus/es/components/dropdown/index'
import { ElEmpty } from 'element-plus/es/components/empty/index'
import { ElForm } from 'element-plus/es/components/form/index'
import { ElFormItem } from 'element-plus/es/components/form/index'
import { ElIcon } from 'element-plus/es/components/icon/index'
import { ElInput } from 'element-plus/es/components/input/index'
import { ElInputNumber } from 'element-plus/es/components/input-number/index'
import { ElOption } from 'element-plus/es/components/select/index'
import { ElPagination } from 'element-plus/es/components/pagination/index'
import { ElProgress } from 'element-plus/es/components/progress/index'
import { ElRadioButton } from 'element-plus/es/components/radio/index'
import { ElRadioGroup } from 'element-plus/es/components/radio/index'
import { ElSelect } from 'element-plus/es/components/select/index'
import { ElSkeleton } from 'element-plus/es/components/skeleton/index'
import { ElSwitch } from 'element-plus/es/components/switch/index'
import { ElTable } from 'element-plus/es/components/table/index'
import { ElTableColumn } from 'element-plus/es/components/table/index'
import { ElTabPane } from 'element-plus/es/components/tabs/index'
import { ElTabs } from 'element-plus/es/components/tabs/index'
import { ElTag } from 'element-plus/es/components/tag/index'
import { ElTooltip } from 'element-plus/es/components/tooltip/index'
import { ElLoading } from 'element-plus/es/components/loading/index'
// Base reset + per-component styles (on-demand replacement for dist/index.css).
import 'element-plus/theme-chalk/base.css'
import 'element-plus/es/components/alert/style/css'
import 'element-plus/es/components/button/style/css'
import 'element-plus/es/components/card/style/css'
import 'element-plus/es/components/checkbox/style/css'
import 'element-plus/es/components/collapse/style/css'
import 'element-plus/es/components/collapse-item/style/css'
import 'element-plus/es/components/dialog/style/css'
import 'element-plus/es/components/divider/style/css'
import 'element-plus/es/components/drawer/style/css'
import 'element-plus/es/components/dropdown/style/css'
import 'element-plus/es/components/dropdown-item/style/css'
import 'element-plus/es/components/dropdown-menu/style/css'
import 'element-plus/es/components/empty/style/css'
import 'element-plus/es/components/form/style/css'
import 'element-plus/es/components/form-item/style/css'
import 'element-plus/es/components/icon/style/css'
import 'element-plus/es/components/input/style/css'
import 'element-plus/es/components/input-number/style/css'
import 'element-plus/es/components/loading/style/css'
import 'element-plus/es/components/message/style/css'
import 'element-plus/es/components/message-box/style/css'
import 'element-plus/es/components/option/style/css'
import 'element-plus/es/components/pagination/style/css'
import 'element-plus/es/components/progress/style/css'
import 'element-plus/es/components/radio-button/style/css'
import 'element-plus/es/components/radio-group/style/css'
import 'element-plus/es/components/select/style/css'
import 'element-plus/es/components/skeleton/style/css'
import 'element-plus/es/components/switch/style/css'
import 'element-plus/es/components/table/style/css'
import 'element-plus/es/components/table-column/style/css'
import 'element-plus/es/components/tab-pane/style/css'
import 'element-plus/es/components/tabs/style/css'
import 'element-plus/es/components/tag/style/css'
import 'element-plus/es/components/tooltip/style/css'
// EP's own dark-theme variables (html.dark). Load before our layers so the
// ep-theme.css html.dark block wins the cascade (ui-guidelines §2).
import 'element-plus/theme-chalk/dark/css-vars.css'
// Token layers must load after the Element Plus stylesheets to win the
// cascade (ui-guidelines §2): raw scales → semantic tokens → EP mapping.
import './styles/tokens.css'
import './styles/semantics.css'
import './styles/ep-theme.css'
// Print layout for report export (ticket 33); global, not a component style.
import './styles/print.css'
import router from './router'
import App from './App.vue'

// v1 dark-mode residue cleanup (GH #112, spec 0018 decision 10): the theme
// toggle (utils/theme.ts) and the anti-FOUC script are gone, but a v1
// session may have left the `hs:dark` localStorage key and the html.dark
// class behind — scrub both once at boot so the light-first rebuild never
// forks on a stale preference. The dark spec reintroduces its own flow.
document.documentElement.classList.remove('dark')
try {
  localStorage.removeItem('hs:dark')
} catch {
  /* storage unavailable (private mode) — nothing persisted anyway */
}

const app = createApp(App)
app.use(router)
const epComponents = [
  ElAlert,
  ElButton,
  ElCard,
  ElCheckbox,
  ElCollapse,
  ElCollapseItem,
  ElDialog,
  ElDivider,
  ElDrawer,
  ElDropdown,
  ElDropdownItem,
  ElDropdownMenu,
  ElEmpty,
  ElForm,
  ElFormItem,
  ElIcon,
  ElInput,
  ElInputNumber,
  ElOption,
  ElPagination,
  ElProgress,
  ElRadioButton,
  ElRadioGroup,
  ElSelect,
  ElSkeleton,
  ElSwitch,
  ElTable,
  ElTableColumn,
  ElTabPane,
  ElTabs,
  ElTag,
  ElTooltip,
]
for (const component of epComponents) app.use(component)
// v-loading directive + Loading service (14 views use the directive).
app.use(ElLoading)
app.mount('#app')
