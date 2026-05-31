import { defineConfig } from 'vite'
import react from '@vitejs/plugin-react'

// https://vite.dev/config/
export default defineConfig({
  build: {
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
  plugins: [react()],
})
