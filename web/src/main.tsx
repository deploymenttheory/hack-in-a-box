import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import './styles.css'

const { App } = import.meta.env.MODE === 'admin' ? await import('./app') : await import('./participant-app')
createRoot(document.getElementById('root')!).render(<StrictMode><App /></StrictMode>)
