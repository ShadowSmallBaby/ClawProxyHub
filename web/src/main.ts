import { createApp } from 'vue'
import { createPinia } from 'pinia'
import App from './App.vue'
import i18n from './i18n'
import router from './router'
import 'tdesign-vue-next/es/style/index.css'
import './assets/theme.css'

createApp(App).use(createPinia()).use(router).use(i18n).mount('#app')

// 仅完整应用页面确认可信资源已启动；沙箱扩展不获得平台桥。
if (window.top===window) {
  const platform=(window as unknown as {cphPlatform?:{postMessage:(data:string)=>void}}).cphPlatform
  platform?.postMessage(JSON.stringify({type:'frontend-ready'}))
}
