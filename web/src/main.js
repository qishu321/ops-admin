import { createApp } from 'vue'
import ElementPlus from 'element-plus'
import 'element-plus/dist/index.css'
import * as ElementPlusIconsVue from '@element-plus/icons-vue'

import App from './App.vue'
import router from './router'
import './style.css'
import { getPermissions } from './utils/auth'
import { createPermissionDirective } from './utils/permission-directive'
import { applySystemTheme, getSystemConfig } from './utils/system-config'

const app = createApp(App)

applySystemTheme(getSystemConfig())

for (const [key, component] of Object.entries(ElementPlusIconsVue)) {
  app.component(key, component)
}

app.directive('permission', createPermissionDirective(getPermissions))

app.use(router)
app.use(ElementPlus)
app.mount('#app')
