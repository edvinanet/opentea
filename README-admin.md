# Admin GUI, roles, and the `/admin/v1` API

This covers everything added on top of the base server (see `README.md` and `SMOKE_TEST.md`
for the TEA consumer read API itself): user accounts, roles, the browser GUI at `/admin/ui`,
the full `/admin/v1` JSON API (now authenticated), and API keys for `/tea/v1`.

## 1. Bootstrapping the first admin

The `user` table starts empty, so there's a dedicated CLI subcommand to create the first admin
(and any subsequent one, if you ever need to create an admin outside the GUI):

```bash
go run ./cmd/opentea createadmin -username=admin -password=<a-real-password>
```

This opens the same database the server would (`TEA_DB_PATH`, default `data/opentea.db`),
creates an `admin`-role user, and exits. Run it before starting the server for the first time.
Once at least one admin exists, more users (admin or consumer) can be created from the GUI's
Users page or via `POST /admin/v1/users`.

## 2. Roles

| Role | Grants |
|---|---|
| `admin` | Everything `consumer` can do, plus: create/update/delete data via `/admin/v1` (products, releases, components, collections, artifacts, CLE events, file uploads), manage users. |
| `consumer` | Read-only: GET endpoints under `/admin/v1`, the dashboard, and the "My API Key" page. Cannot write data, manage users, or view the Users page. |

Both roles can log into the GUI and generate their own `/tea/v1` API key — that's identity,
not a privilege escalation, since `/tea/v1` is a read-only API regardless of who's calling it.

An admin can't delete the last remaining admin user (`DELETE .../users/{uuid}` returns `400`
in that case) — there's always at least one way back in.

## 3. Web GUI (`/admin/ui/...`)

Session-cookie based; a plain HTML login form, no JavaScript.

| Route | Method | Role | Purpose |
|---|---|---|---|
| `/admin/ui/login` | GET, POST | — | Login form / submit |
| `/admin/ui/logout` | POST | any | Ends the session |
| `/admin/ui/` | GET | consumer+ | Dashboard: counts of products, product releases, components, component releases, collections, artifacts |
| `/admin/ui/users` | GET, POST | admin | List + create users |
| `/admin/ui/users/{uuid}/delete` | POST | admin | Delete a user |
| `/admin/ui/token` | GET | consumer+ | Shows whether you have an API key and when it was created (never the secret itself) |
| `/admin/ui/token/generate` | POST | consumer+ | (Re)generates your API key (a key ID + secret pair); both are shown **once**, immediately after generation — copy them then, the secret can't be retrieved again |

Session cookies (`opentea_session`) are `HttpOnly`, `SameSite=Lax`, and expire 24h after login
(fixed, not renewed on activity — log in again after that).

Every page's nav bar shows a "Default TEA" / "Trusted TEA" badge reflecting the
`TEA_TRUST_ARCHITECTURE` config setting (see `README.md`'s config table) — this deployment's
declared profile, display-only (it doesn't enforce anything). Independently, each artifact and
collection on a product/component release's detail page shows its own trust-architecture evidence
status (Signed / Draft / no badge for "none attached"), based on whether an evidence bundle has
actually been attached via `POST /admin/v1/artifacts/{uuid}/{version}/evidenceBundle` or
`POST /admin/v1/collections/{uuid}/{version}/evidenceBundle` — independent of the nav bar's
deployment-wide setting, since evidence can exist (or be missing) regardless of what's declared.

## 4. `/admin/v1` JSON API

**Every** endpoint now requires a valid session cookie (`opentea_session`) — there is no
separate API-key auth for this API; it's the same cookie the GUI uses. To call it with `curl`,
log in first and reuse the cookie jar:

```bash
curl -c cookies.txt -X POST http://localhost:8080/admin/ui/login \
  -d 'username=admin&password=<password>'
curl -b cookies.txt http://localhost:8080/admin/v1/products
```

GET routes require `consumer` or higher; POST/DELETE routes require `admin`. A missing/invalid
session gets `401`; an authenticated-but-under-privileged request gets `403`.

### Products

| Method | Path | Role |
|---|---|---|
| POST | `/admin/v1/products` | admin |
| GET | `/admin/v1/products` | consumer |
| GET | `/admin/v1/products/{uuid}` | consumer |
| DELETE | `/admin/v1/products/{uuid}` | admin |
| POST | `/admin/v1/products/{uuid}/releases` | admin |
| POST | `/admin/v1/products/{uuid}/cle/events` | admin |
| POST | `/admin/v1/products/{uuid}/cle/definitions` | admin |

```bash
curl -b cookies.txt -X POST $BASE/admin/v1/products -H 'Content-Type: application/json' -d '{
  "name": "Acme Widget",
  "identifiers": [{"idType": "PURL", "idValue": "pkg:generic/acme-widget"}]
}'
```

### Product releases

| Method | Path | Role |
|---|---|---|
| GET | `/admin/v1/productReleases/{uuid}` | consumer |
| DELETE | `/admin/v1/productReleases/{uuid}` | admin |
| POST | `/admin/v1/productReleases/{uuid}/components` | admin |
| POST | `/admin/v1/productReleases/{uuid}/collections` | admin |
| POST | `/admin/v1/productReleases/{uuid}/cle/events` | admin |
| POST | `/admin/v1/productReleases/{uuid}/cle/definitions` | admin |

```bash
curl -b cookies.txt -X POST $BASE/admin/v1/productReleases/$PRODUCT_RELEASE_UUID/components \
  -H 'Content-Type: application/json' -d "{\"uuid\": \"$COMPONENT_UUID\", \"release\": \"$COMPONENT_RELEASE_UUID\"}"
```

### Components

| Method | Path | Role |
|---|---|---|
| POST | `/admin/v1/components` | admin |
| GET | `/admin/v1/components` | consumer |
| GET | `/admin/v1/components/{uuid}` | consumer |
| DELETE | `/admin/v1/components/{uuid}` | admin |
| POST | `/admin/v1/components/{uuid}/releases` | admin |
| POST | `/admin/v1/components/{uuid}/cle/events` | admin |
| POST | `/admin/v1/components/{uuid}/cle/definitions` | admin |

### Component releases

| Method | Path | Role |
|---|---|---|
| GET | `/admin/v1/componentReleases/{uuid}` | consumer |
| DELETE | `/admin/v1/componentReleases/{uuid}` | admin |
| POST | `/admin/v1/componentReleases/{uuid}/distributions` | admin |
| POST | `/admin/v1/componentReleases/{uuid}/collections` | admin |
| POST | `/admin/v1/componentReleases/{uuid}/cle/events` | admin |
| POST | `/admin/v1/componentReleases/{uuid}/cle/definitions` | admin |

### Distributions and artifacts (file uploads)

| Method | Path | Role |
|---|---|---|
| POST | `/admin/v1/distributions/{id}/files` | admin |
| POST | `/admin/v1/artifacts` | admin |
| POST | `/admin/v1/artifacts/{uuid}/{version}/files?formatIndex=N` | admin |
| POST | `/admin/v1/artifacts/{uuid}/{version}/signature?formatIndex=N` | admin |

Uploaded artifact content/signatures are never re-exposed as a `url`/`signatureUrl` on the
artifact (TEA 1.0, `spec/openapi.yaml`: those fields are reserved for genuinely external
locations) — retrieve self-hosted content via the public download endpoints instead:
`GET /tea/v1/artifact/{uuid}/{latest|version}/download` and its `.../signature/download`
counterpart, both `mediaType`-selected the same way as `/files`.

```bash
curl -b cookies.txt -X POST $BASE/admin/v1/distributions/$DISTRIBUTION_ID/files \
  -F "file=@widget.tar.gz;type=application/gzip"
```

### CLE (lifecycle events and support definitions)

Available for all four owner types (products, product releases, components, component
releases) at `.../cle/events` and `.../cle/definitions` — see the tables above. All admin-only.

### Users

| Method | Path | Role |
|---|---|---|
| POST | `/admin/v1/users` | admin |
| GET | `/admin/v1/users` | consumer |
| DELETE | `/admin/v1/users/{uuid}` | admin |

```bash
curl -b cookies.txt -X POST $BASE/admin/v1/users -H 'Content-Type: application/json' -d '{
  "username": "viewer", "password": "a-real-password", "role": "consumer"
}'
```

Responses never include `password_hash` or any secret — the `user` JSON shape is
`{"uuid", "username", "role", "createdAt"}` only.

### Stats (also the monitoring endpoint)

| Method | Path | Role |
|---|---|---|
| GET | `/admin/v1/stats` | consumer |

```bash
curl -b cookies.txt $BASE/admin/v1/stats
# {"startedAt":"2026-07-04T09:00:00Z","orgName":"Acme Corp","products":2,"productReleases":3,"components":1,"componentReleases":1,"collections":1,"artifacts":1}
```

`startedAt` is the server process's own start time (not query time) — use it for uptime
checks/monitoring. It's set once at process startup and doesn't change until the process
restarts. `orgName` reflects the `TEA_ORG_NAME` config value and is omitted entirely when unset.

`collections` and `artifacts` count distinct identities (by `uuid`), not every revision —  a
component release with three collection versions still counts as one collection.

### Product import/export bundles

| Method | Path | Role |
|---|---|---|
| GET | `/admin/v1/products/{uuid}/export` | **admin** (not consumer) |
| POST | `/admin/v1/products/import` | **admin** (not consumer) |

For backup, transferring product ownership between organizations, or migrating between hosted
TEA providers — entirely separate from `/tea/v1` and any future publisher API. Full format
specification: [docs/bundle-format.md](docs/bundle-format.md).

```bash
curl -b cookies.txt $BASE/admin/v1/products/$PRODUCT_UUID/export -o product.zip
curl -b cookies.txt -X POST $BASE/admin/v1/products/import -F bundle=@product.zip
# {"ProductCreated":true,"Created":{"product":1,"productRelease":2,...},"AlreadyExisted":{}}
```

Import is idempotent — re-importing the same bundle, or a bundle that overlaps data you already
have (e.g. two products sharing a component), never creates duplicates; `AlreadyExisted` in the
response tells you what was already there.

### Publisher credentials (`/publisher/v1` bearer tokens)

`/publisher/v1` (opentea's server-side implementation of the draft standard TEA Publisher
API — `design/publisher-openapi.yaml`, `design/publisher-service.md`, `internal/publisher`)
is authenticated by its own bearer-token credential, not the admin session cookie. Issue and
manage those credentials here:

| Method | Path | Role |
|---|---|---|
| POST | `/admin/v1/publisherCredentials` | admin |
| GET | `/admin/v1/publisherCredentials` | consumer |
| DELETE | `/admin/v1/publisherCredentials/{uuid}` | admin |

Two scopes, structurally enforced (`design/publisher-service.md` §10.4): `full` may call
every `/publisher/v1` operation; `cicd` may create/upload/validate artifacts and drive
collection-draft assembly and the mechanical prepare/cancel/commit steps, but may never
create products/components/releases/CLE events, or approve/reject a collection draft — issue
a CI/CD pipeline a `cicd`-scoped credential, and a publisher platform's own service identity
a `full`-scoped one.

```bash
curl -b cookies.txt -X POST $BASE/admin/v1/publisherCredentials -H 'Content-Type: application/json' -d '{
  "label": "acme-ci-pipeline", "scope": "cicd"
}'
# {"uuid":"...","label":"acme-ci-pipeline","scope":"cicd","createdAt":"...","token":"..."}
```

The raw `token` is returned **only in this response** — store it now; it can't be retrieved
again (revoke and reissue if it's lost). Use it as `Authorization: Bearer <token>` against
`/publisher/v1`:

```bash
curl -H "Authorization: Bearer $TOKEN" -X POST $BASE/publisher/v1/artifacts \
  -H 'Content-Type: application/json' -d '{"type":"BOM","formats":[{"mediaType":"application/vnd.cyclonedx+json"}]}'
```

## 5. Using an API key against `/tea/v1`

`/tea/v1` (the spec-conformant consumer read API) stays fully public — no login, no key,
required. What's new: if you *do* send an `Authorization: Bearer` header, it now has to carry
a valid, unexpired access token (obtained from `POST {TEA_API_BASE_PATH}/token`, below), or the
request is rejected with `401`. This lets you call the read API as a named identity if you want
to (e.g. for audit trails, or future per-identity rate limiting/scoping), without requiring it.

TEA 1.0 requires a real token-exchange step (`spec/openapi.yaml`'s `/token`): your long-lived
API key is never itself accepted as a bearer credential on `/tea/v1` — only a short-lived access
token obtained by exchanging it is.

1. Log into the GUI, go to "API Key", click Generate. Copy the **key ID** and **secret** shown —
   the secret is not retrievable again after that (only the key's creation date is, for future
   reference; the key ID itself is safe to note down too, but isn't re-shown either).
2. Exchange the key for an access token, using HTTP Basic (key ID as the username, secret as the
   password) and the `client_credentials` grant:
   ```bash
   curl -u <keyId>:<secret> -d grant_type=client_credentials $BASE/tea/v1/token
   # {"access_token":"...","token_type":"Bearer","expires_in":3600}
   ```
3. Use the returned `access_token` — not the key itself:
   ```bash
   curl -H "Authorization: Bearer <access_token>" $BASE/tea/v1/products
   ```
4. Calling `/tea/v1` with no `Authorization` header at all still works exactly as before —
   this is purely additive.
5. An access token expires after `TEA_ACCESS_TOKEN_TTL` (default 1h; see `README.md`'s config
   table) — re-exchange your key at step 2 for a new one. TEA defines no refresh token, so
   there's nothing else to do: a client holding its own key can simply call `/token` again.
6. Regenerating your key immediately invalidates the previous key/secret pair (any access
   tokens already issued from it keep working until they expire, since they're a separate
   credential). There's one key per user.

## 6. Security posture (Phase 1 of this feature)

- Passwords are hashed with bcrypt (`golang.org/x/crypto/bcrypt`, default cost). Never stored
  or logged in plaintext.
- API key secrets and access tokens are stored as a SHA-256 hash, not the raw value — a database
  leak doesn't hand out usable credentials, same principle as the password hashing (though a
  faster hash is fine here since these are high-entropy random values, not user-chosen
  low-entropy secrets). A key's ID is not confidential and is stored/displayed as plain text.
- Session cookies are `HttpOnly` (not readable from JS) and `SameSite=Lax` (not sent on
  cross-site POSTs), which is the CSRF mitigation for this phase. A dedicated per-form CSRF
  token is a tracked follow-up (see `TODO.md`), not built yet.
- `VerifyLogin` returns the same error for "no such user" and "wrong password" so failed
  logins can't be used to enumerate valid usernames.
- Deleting the last admin is blocked at the repo layer, so the system can never end up with
  zero admins able to log in.
