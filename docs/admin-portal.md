# Independent admin portal

Run the normal Compose stack with `docker compose up -d --build --wait api admin worker` after migrations. The participant app is at `http://127.0.0.1:8080`; administration is at `http://127.0.0.1:8081`. Configure `HACKBOX_ADMIN_PORT` and `HACKBOX_ADMIN_ORIGIN` to change the admin address. Use separate hostnames and independently routed services in production.

The `public` and `admin` Docker targets copy separate frontend artifacts into separate API processes. Neither process proxies API requests or assets through the other. The participant build has registration, event dates, resources, deliverables and account pages; event creation, templates and organiser workspaces belong to the admin build. Both API processes call the same domain services with the same current role, CSRF and revision checks. An admin URL never grants authority.

Register both origins' `/auth/callback` URLs with the identity provider, or configure a separate admin client with `HACKBOX_ADMIN_OIDC_CLIENT_ID` and `HACKBOX_ADMIN_OIDC_CLIENT_SECRET`. Login and session cookies have separate deployment names, including on localhost where ports do not isolate cookies. Existing combined-app sessions require a new sign-in. For GitHub linking, register each origin's `/auth/github/callback` too. No authentication bypass is provided.

For development, `npm --prefix web run dev` serves participants and `npm --prefix web run dev:admin` serves administration. Point each Vite proxy at its corresponding API process. `npm --prefix web run build` produces `web/dist` and `web/dist-admin`; existing organiser browser checks use the admin artifact.

`sh scripts/smoke-compose.sh` creates an isolated database and file volume, stops the public app, and verifies admin health, database readiness, deep links, protected API response and authenticated operational metrics. It then verifies restart persistence and removes only its synthetic project. Its uniquely tagged images do not replace a running preview image.

Shared database, file storage, identity provider and Docker host failures can still affect both apps. Schema changes must remain compatible with both deployments. Independent containers provide process and release isolation, not disaster recovery for shared infrastructure.
