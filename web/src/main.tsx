import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import './styles.css'

const { App } = import.meta.env.MODE === 'admin' ? await import('./app') : window.location.pathname.startsWith('/display/') ? await import('./room-display') : await import('./participant-app')
createRoot(document.getElementById('root')!).render(<StrictMode><App /></StrictMode>)
