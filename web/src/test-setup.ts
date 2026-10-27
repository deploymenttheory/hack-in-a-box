import '@testing-library/jest-dom/vitest'
import { cleanup } from '@testing-library/react'
import { afterEach } from 'vitest'
afterEach(cleanup)

// Native responsive collections use matchMedia; jsdom does not implement it.
Object.defineProperty(window, 'matchMedia', { writable: true, value: (query: string) => ({ matches: false, media: query, onchange: null, addEventListener() {}, removeEventListener() {}, dispatchEvent() { return false } }) })

// jsdom has no layout or ResizeObserver; responsive geometry/focus is verified in Chromium.
class TestResizeObserver { observe() {} unobserve() {} disconnect() {} }
Object.defineProperty(window, 'ResizeObserver', { writable: true, value: TestResizeObserver })
