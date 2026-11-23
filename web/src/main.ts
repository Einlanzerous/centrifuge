import { createApp } from 'vue'
import App from './App.vue'
// Fonts are self-hosted (no request leaves for Google Fonts).
import '@fontsource-variable/geist'
import '@fontsource-variable/geist-mono'
import '@fontsource/instrument-serif/latin-400.css'
import '@fontsource/instrument-serif/latin-400-italic.css'
import './style.css'

createApp(App).mount('#app')
