import { createContext, useContext, useState, type CSSProperties, type ReactNode } from 'react'

const PortalContext = createContext<HTMLElement | null>(null)
export const useThemePortal = () => useContext(PortalContext)

// A portal stays inside the event's theme scope. Never apply event branding to body.
export function Theme({ children, accent, style, mode = 'system', density = 'comfortable' }: { children: ReactNode; accent?: string; style?: CSSProperties; mode?: 'light' | 'dark' | 'system'; density?: 'comfortable' | 'compact' }) {
  const [portal, setPortal] = useState<HTMLDivElement | null>(null)
  return <div className="theme" data-theme={mode} data-density={density} style={{ ...(accent ? { '--primary': accent } : {}), ...style } as CSSProperties}>
    <PortalContext.Provider value={portal}>{children}<div ref={setPortal} data-theme-portals="" /></PortalContext.Provider>
  </div>
}
