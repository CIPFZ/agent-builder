import React from 'react';
import ReactDOM from 'react-dom/client';
import { ConfigProvider } from 'antd';
import zhCN from 'antd/locale/zh_CN';
import App from './app/App.tsx';
import { AppErrorBoundary } from './app/AppErrorBoundary.tsx';
import './styles.css';

ReactDOM.createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    <ConfigProvider locale={zhCN}>
      <AppErrorBoundary>
        <App />
      </AppErrorBoundary>
    </ConfigProvider>
  </React.StrictMode>,
);
