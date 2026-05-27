import React from 'react'
import ReactDOM from 'react-dom/client'
import '@arco-design/web-react/dist/css/arco.css'
import App from '@/App.tsx'
import './index.css'
import './styles/arco-dark.css'
import { ArcoAppProvider } from '@/providers/ArcoAppProvider'
import I18nSync from '@/i18n/I18nSync'

ReactDOM.createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    <ArcoAppProvider>
      <I18nSync />
      <App />
    </ArcoAppProvider>
  </React.StrictMode>
)
