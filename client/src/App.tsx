import AntApp from 'antd/es/app'
import ConfigProvider from 'antd/es/config-provider'
import theme from 'antd/es/theme'
import { AssistantClient } from './AssistantClient'
import './App.css'

function App() {
  return (
    <ConfigProvider
      theme={{
        algorithm: theme.defaultAlgorithm,
        token: {
          colorPrimary: '#d97757',
          borderRadius: 8,
          fontFamily: 'Inter, ui-sans-serif, system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif',
        },
      }}
    >
      <AntApp>
        <AssistantClient />
      </AntApp>
    </ConfigProvider>
  )
}

export default App
