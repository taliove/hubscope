import { createApp } from 'vue'
import ElementPlus from 'element-plus'
import 'element-plus/dist/index.css'
// Token overrides must load after the Element Plus stylesheet to win the
// cascade (spec: docs/specs/0003-ui-redesign.md §3.1).
import './styles/tokens.css'
import router from './router'
import App from './App.vue'

const app = createApp(App)
app.use(router)
app.use(ElementPlus)
app.mount('#app')
