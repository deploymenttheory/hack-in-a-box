import { defineConfig } from '@playwright/test'
import base from './playwright.config'
export default defineConfig({ ...base, outputDir: 'test-results-public', testIgnore: [], testMatch: ['objective-disclosure.spec.ts', 'feedback-reader.spec.ts', 'scorecard-workspace.spec.ts', 'public-portal.spec.ts', 'participant-teams.spec.ts', 'participant-deliverables.spec.ts', 'registration.spec.ts', 'offers.spec.ts', 'room-display.spec.ts', 'team-access.spec.ts', 'inbox.spec.ts', 'participant-guidance.spec.ts', 'email-account.spec.ts', 'help-reader.spec.ts', 'help-workspace.spec.ts', 'connection-recovery.spec.ts', 'mentor-workspace.spec.ts', 'clarification-reader.spec.ts', 'clarification-workspace.spec.ts', 'delivery-history.spec.ts', 'automation-recovery.spec.ts'],
  use: { ...base.use, baseURL: 'http://127.0.0.1:4274' },
  webServer: { command: 'npm run preview -- --port 4274', port: 4274, reuseExistingServer: false },
})
