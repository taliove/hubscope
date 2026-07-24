import { createApp } from 'vue'
import ElementPlus from 'element-plus'
import 'element-plus/dist/index.css'
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

const app = createApp(App)
app.use(router)
app.use(ElementPlus)
app.mount('#app')
