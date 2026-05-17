import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

// https://vite.dev/config/
export default defineConfig({
  build: {
    rollupOptions: {
      output: {
        manualChunks(id) {
          if (id.includes('node_modules/react') || id.includes('node_modules/react-dom')) {
            return 'react'
          }
          if (id.includes('node_modules/@ant-design/x/es/bubble')) {
            return 'ant-design-x-bubble'
          }
          if (id.includes('node_modules/@ant-design/x/es/sender')) {
            return 'ant-design-x-sender'
          }
          if (id.includes('node_modules/@ant-design/x/es/thought-chain')) {
            return 'ant-design-x-thought-chain'
          }
          if (id.includes('node_modules/@ant-design/x')) {
            return 'ant-design-x-shared'
          }
          if (id.includes('node_modules/antd') || id.includes('node_modules/@ant-design/icons')) {
            return 'antd'
          }
          return undefined
        },
      },
    },
  },
  plugins: [react()],
})
