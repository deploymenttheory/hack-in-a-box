import { createContext } from 'react'
// Presentation-only coordination: selected detail and collection share one
// visible state message, including when the collection is hidden on mobile.
export const WorkspaceFeedbackContext = createContext(false)
