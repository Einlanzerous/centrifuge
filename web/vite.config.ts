import { defineConfig } from 'vite'
import vue from '@vitejs/plugin-vue'

// In dev, proxy the API to the local backend (construct-server runs centrifuge
// on :4003) so the SPA can call same-origin /api and /feed.xml without CORS.
// Override with CENTRIFUGE_API_PROXY when the backend lives elsewhere.
const apiTarget = process.env.CENTRIFUGE_API_PROXY || 'http://localhost:4003'

export default defineConfig({
  plugins: [vue()],
  server: {
    // Bind all interfaces so the dev server is reachable from other machines on
    // the LAN / tailnet (this runs on the homelab box, not a dev laptop).
    host: true,
    port: 5173,
    // Vite blocks requests whose Host header isn't allowlisted (DNS-rebinding
    // protection). Permit the homelab hostname and Tailscale MagicDNS so the
    // server is reachable as http://imperial-construct:5173 / *.ts.net.
    // Override with CENTRIFUGE_ALLOWED_HOSTS (comma-separated) if needed.
    allowedHosts: process.env.CENTRIFUGE_ALLOWED_HOSTS?.split(',') ?? [
      'localhost',
      'imperial-construct',
      '.ts.net',
    ],
    proxy: {
      '/api': { target: apiTarget, changeOrigin: true },
      '/feed.xml': { target: apiTarget, changeOrigin: true },
    },
  },
})

