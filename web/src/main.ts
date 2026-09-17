import { createApp } from 'vue'
import { createPinia } from 'pinia'
import TDesign from 'tdesign-vue-next'
import App from './App.vue'
import i18n from './i18n'
import router from './router'
import 'tdesign-vue-next/es/style/index.css'
import './assets/theme.css'

createApp(App).use(createPinia()).use(router).use(TDesign).use(i18n).mount('#app')
