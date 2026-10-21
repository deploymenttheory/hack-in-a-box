import { createContext, useContext, useState, type CSSProperties, type ReactNode } from 'react'

const PortalContext = createContext<HTMLElement | null>(null)
export const useThemePortal = () => useContext(PortalContext)

// A portal stays inside the event's theme scope. Never apply event branding to body.
export function Theme({ children, accent, style }: { children: ReactNode; accent?: string; style?: CSSProperties }) {
  const [portal, setPortal] = useState<HTMLDivElement | null>(null)
  return <div className="theme" style={{ ...(accent ? { '--primary': accent } : {}), ...style } as CSSProperties}>
    <PortalContext.Provider value={portal}>{children}<div ref={setPortal} data-theme-portals="" /></PortalContext.Provider>
  </div>
}
