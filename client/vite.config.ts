import { defineConfig, type ViteDevServer } from 'vite'
import react from '@vitejs/plugin-react'
import { existsSync, readFileSync } from 'node:fs'
import type { IncomingMessage, ServerResponse } from 'node:http'
import { extname, normalize, resolve, sep } from 'node:path'

const desktopBindingsDir = resolve(__dirname, '../desktop/frontend/bindings')
const runtimeProxyTarget = process.env.VITE_AGENT_BUILDER_RUNTIME_URL || 'http://127.0.0.1:5183'

function devBindingsPlugin() {
  return {
    name: 'agent-builder-dev-bindings',
    configureServer(server: ViteDevServer) {
      server.middlewares.use('/bindings', (req: IncomingMessage, res: ServerResponse, next: () => void) => {
        const requestPath = decodeURIComponent(req.url?.split('?')[0] ?? '')
        const filePath = resolve(desktopBindingsDir, `.${requestPath}`)
        const normalizedRoot = normalize(desktopBindingsDir + sep)
        const normalizedFile = normalize(filePath)

        if (!normalizedFile.startsWith(normalizedRoot) || !existsSync(filePath)) {
          next()
          return
        }

        const contentType = extname(filePath) === '.js' ? 'application/javascript' : 'application/octet-stream'
        res.setHeader('Content-Type', contentType)
        res.end(readFileSync(filePath))
      })
    },
  }
}

// https://vite.dev/config/
export default defineConfig({
  server: {
    proxy: {
      '/runtime-api': {
        target: runtimeProxyTarget,
        changeOrigin: true,
        rewrite: (path) => path.replace(/^\/runtime-api/, ''),
      },
    },
  },
  build: {
    target: 'chrome109',
    chunkSizeWarningLimit: 1200,
    rolldownOptions: {
      output: {
        codeSplitting: {
          includeDependenciesRecursively: false,
          groups: [
            {
              name: 'react',
              test: /node_modules[\\/](react|react-dom|scheduler)[\\/]/,
              priority: 90,
            },
            {
              name: 'ant-design-icons',
              test: /node_modules[\\/]@ant-design[\\/]icons[\\/]/,
              priority: 80,
            },
            {
              name: 'ant-design-x-sender',
              test: /node_modules[\\/]@ant-design[\\/]x[\\/]es[\\/]sender[\\/]/,
              priority: 75,
            },
            {
              name: 'ant-design-x-shared',
              test: /node_modules[\\/]@ant-design[\\/]x[\\/]/,
              priority: 74,
            },
            {
              name: 'antd-navigation',
              test: /node_modules[\\/](antd[\\/]es[\\/](layout|menu|radio|select|switch|card)|rc-menu|rc-select|rc-trigger|rc-overflow|rc-virtual-list|rc-motion)[\\/]/,
              priority: 72,
            },
            {
              name: 'antd-shared',
              test: /node_modules[\\/](antd|@ant-design|@rc-component|rc-)[\\/]/,
              priority: 70,
            },
          ],
        },
        strictExecutionOrder: true,
      },
    },
  },
  plugins: [react(), devBindingsPlugin()],
})
