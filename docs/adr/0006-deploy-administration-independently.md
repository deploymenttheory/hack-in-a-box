# Deploy administration independently of the participant application

The participant application and admin portal use separate frontend builds, origins, session cookies and API processes so a failed or stopped participant deployment does not prevent administration. Both processes use the same domain services and retain current permission checks; visiting the admin origin grants no additional authority. This trades an additional deployment and identity-provider callback configuration for independent availability and releases.

PostgreSQL, file storage and the identity provider remain shared dependencies. Independent processes do not protect against failure of those services, an incompatible schema migration, or exhaustion of a shared host; production deployments must provision resources and route each origin independently.
