# Teams Workflows broadcast setup

Use the Teams Workflows app with **When a Teams webhook request is received**, posting an Adaptive Card to an approved event channel. The adapter sends the documented `message`/`attachments` Adaptive Card envelope. This trigger supports POST and has separate Anyone, tenant-user and selected-user authentication modes. This integration supports **Anyone with the secret URL**; it does not send an Entra bearer token. Tenant-authenticated triggers require a separate OAuth integration and are unavailable here. [Microsoft Teams connector contract](https://learn.microsoft.com/en-us/connectors/teams/).

Open Workflows from the intended channel, choose an incoming webhook channel template, select the destination team/channel and save it. Copy the URL from the saved trigger into the event's private Teams configuration. Do not paste it into a campaign, ticket, chat, screenshot or log. It is a credential. Microsoft documents the setup and the distinction between channel/chat templates. [Microsoft setup instructions](https://support.microsoft.com/en-us/workflows/send-messages-in-teams-using-incoming-webhooks).

Give the workflow an owner and at least one co-owner with access to the destination channel and connections. Check their continued membership and the flow's enabled state before the event. Workflows belong to individual owners rather than to the team/channel, so a sole owner's departure can orphan the flow. [Microsoft ownership guidance](https://learn.microsoft.com/en-us/microsoftteams/platform/webhooks-and-connectors/how-to/add-incoming-webhook).

## Supported destination policy

Use the **current URL from the trigger designer**. Microsoft retired the previous `logic.azure.com` HTTP/Teams webhook trigger URLs after 30 November 2025. Newly generated URLs can exceed 255 characters; configuration accepts up to 4096. [Microsoft trigger migration notice](https://learn.microsoft.com/en-us/troubleshoot/power-platform/power-automate/flow-run-issues/triggers-troubleshoot?tabs=new-designer).

The verified public-cloud shape is HTTPS on the environment Power Platform host with `/powerautomate/automations/direct/workflows/<workflow-id>/triggers/manual/paths/invoke` and `api-version=1`. A Microsoft-owned current sample shows that hostname/path/version; its OAuth authentication differs from the secret-URL mode used here. [Microsoft current endpoint sample](https://github.com/microsoft/CopilotStudioSamples/blob/main/infrastructure/manage-paygo/scripts/UnlinkBillingPolicyRunbook.ps1).

The adapter accepts only the verified environment host suffix and workflow path, with standard signature query fields. It rejects legacy connector domains, arbitrary Microsoft services, custom forwarding hosts, user information, fragments, unexpected ports/query fields and encoded alternate paths. Every attempt resolves the hostname, rejects any private/reserved/mapped answer and pins a validated public address while retaining normal TLS hostname/certificate validation. Environment proxies and redirects are disabled. Sovereign-cloud and other endpoint formats need separately verified policy support; they are not silently redirected or accepted as generic webhooks.

## Connection test and delivery evidence

The organiser's explicit **TEST — channel connection check** posts a synthetic broadcast to the configured channel. It is distinct from the campaign composer's organiser-only self-test, which Teams cannot provide. Confirm the channel with its owner before testing. The public broadcast adapter never accepts attendee-specific feedback, credentials, restricted assessment data or arbitrary person-message payloads.

A 2xx response records **provider acceptance**, not proof that Teams displayed the card or that a person received/read it. Check the workflow run history and look for the TEST card in the intended channel. A disabled/deleted/expired connection should be replaced and retested. Changing configuration changes its opaque revision and requires explicit campaign reapproval before queued sends can use it.

A definite 429 rejection may retry within bounded attempt/rate windows and honors an acceptable Retry-After. A response window beyond the retry policy requires organiser review. A request whose acceptance is uncertain, including a lost post-write response or server error, becomes ambiguous and is not automatically replayed. Reconcile it against workflow history and the visible operation reference before choosing any fresh action.

## Verification status

Automated verification uses synthetic local HTTPS fixtures and real PostgreSQL; it sends no real Teams traffic. **Live test-channel verification is not yet performed.** It requires an explicitly approved channel, its workflow secret supplied through private configuration, a current organiser, an owner/co-owner who can inspect run history, and approval for the visible TEST broadcast. A live success claim must include the observed workflow/card result as well as the provider response.
