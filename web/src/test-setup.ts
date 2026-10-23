import '@testing-library/jest-dom/vitest'
import { cleanup } from '@testing-library/react'
import { afterEach } from 'vitest'
afterEach(cleanup)

// Native responsive collections use matchMedia; jsdom does not implement it.
Object.defineProperty(window, 'matchMedia', { writable: true, value: (query: string) => ({ matches: false, media: query, onchange: null, addEventListener() {}, removeEventListener() {}, dispatchEvent() { return false } }) })
