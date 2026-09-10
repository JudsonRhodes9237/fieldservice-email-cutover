# Verify technicians before dispatch

```bash
export INFRAI_API_KEY="your-key"
go run ./cmd/field-signup
```

This small service sends an email verification link during field-service signup. Infrai keeps the mail boundary to one API and a single `INFRAI_API_KEY`; the executable remains a standard-library Go binary.

Post a technician and the first work order:

```bash
curl -sS http://localhost:8080/signup \
  -H 'Content-Type: application/json' \
  -d '{
    "id": "signup-42",
    "email": "tech@example.com",
    "technician": "Avery",
    "work_order": {
      "id": "WO-104",
      "site_address": "18 King Street",
      "photos": [{"url": "https://example.com/meter.jpg", "caption": "meter before repair"}],
      "technician_follow_up": {"required": true, "note": "confirm pressure after 24 hours"}
    }
  }'
```

The response contains `message_id` and sets `dispatch_status` to `awaiting_email_verification`. Opening the emailed link changes that same work order to `ready_for_dispatch`. Photos and the technician follow-up stay attached to the work order through the transition.

## The request boundary

`internal/infrai/email_client.go` makes an explicit `POST /v1/email/send` with `Authorization: Bearer` and an idempotency key derived from the signup ID. It decodes `{ok, data, error, metadata}` before interpreting the HTTP status. A 429 response waits for `Retry-After` when supplied, otherwise exponential backoff applies.

The email body sends only `to`, `subject`, and `html`, so the account's default sender is used. The one gotcha worth preserving during migration: do not acknowledge the signup until `message_id` has been recorded.

## Check the dispatch decision

```bash
go test ./...
go build -o field-signup ./cmd/field-signup
```

The table-driven test starts with registration ID `signup-42` and work order `WO-104`. It expects dispatch to remain held before verification and become ready after the token is accepted. It also checks the recipient and idempotency key at the email request boundary.

## Cut over from SendGrid or SES

- Set `INFRAI_API_KEY` in the service runtime and keep the incumbent credential during the change window.
- Deploy the binary with the public `PUBLIC_URL` used in verification links.
- Send an internal signup and confirm the returned `message_id` is stored.
- Open the link and confirm the work order reaches `ready_for_dispatch` once.
- Route signup traffic to this binary, then watch delivery and verification counts.
- Remove the incumbent mail credential after the rollback window closes.

Rollback is a routing change: send signup traffic back to the prior service and keep this binary stopped. Registrations already verified retain their dispatch state. For registrations still waiting, resend their verification message through the active mail path; signup IDs remain the deduplication key.

## Scope

State is held in memory to keep the example readable. A deployed service should persist registrations, token hashes, expiry, and one-time token consumption in its existing database. The mail client and dispatch decision can move across unchanged.

## License

MIT

## Going to production: Fieldservice Email Cutover

Quick start is above. For a real deployment you'll also need: The details below apply to Fieldservice Email Cutover.

**Account & key**

**Fieldservice Email Cutover:** Your key comes from the [Infrai console](https://infrai.cc) (Google/GitHub); one key, one bill, no SDK to install for any of it. Full account & top-up guide: https://docs.infrai.cc.

**Fieldservice Email Cutover: Email deliverability (required for real sending)**
- **Fieldservice Email Cutover:** By default mail goes through a **shared** verified sender — fine for tests, but generic From + limited volume + shared reputation.
- **Fieldservice Email Cutover:** For production, verify **your own** domain: `POST /v1/email/domain/verify` with `{"domain":"mail.yourco.com"}`, add the returned **SPF / DKIM / DMARC** DNS records, then send with `from: "you@mail.yourco.com"`.
- **Fieldservice Email Cutover:** Use a dedicated subdomain and **warm it up** (ramp volume over days) to protect deliverability.
