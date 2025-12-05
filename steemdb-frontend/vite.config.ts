import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

// https://vite.dev/config/
export default defineConfig({
  plugins: [react()],
  define: {
    // Default environment variables
    // Use relative path for production (Nginx proxy), absolute for development
    'import.meta.env.VITE_API_URL': JSON.stringify(
      process.env.VITE_API_URL || (process.env.NODE_ENV === 'production' ? '/api' : 'http://localhost:8080/api')
    ),
    // For WebSocket, use empty string in production (will use window.location.host), absolute in development
    'import.meta.env.VITE_WS_HOST': JSON.stringify(
      process.env.VITE_WS_HOST || (process.env.NODE_ENV === 'production' ? '' : 'localhost:8080')
    ),
    'import.meta.env.VITE_WS_PATH': JSON.stringify(process.env.VITE_WS_PATH || '/ws'),
    'import.meta.env.VITE_APP_NAME': JSON.stringify(process.env.VITE_APP_NAME || 'SteemDB'),
    'import.meta.env.VITE_APP_VERSION': JSON.stringify(process.env.VITE_APP_VERSION || '1.0.0'),
  },
  server: {
    port: 3000,
    host: true,
  },
  preview: {
    port: 3000,
    host: true,
  },
})
