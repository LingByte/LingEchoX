import React from 'react'
import ReactDOM from 'react-dom/client'
import '@arco-design/web-react/dist/css/arco.css'
import App from '@/App.tsx'
import './index.css'
import { ArcoAppProvider } from '@/providers/ArcoAppProvider'

ReactDOM.createRoot(document.getElementById('root')!).render(
  <React.StrictMode>
    <ArcoAppProvider>
      <App />
    </ArcoAppProvider>
  </React.StrictMode>
)
