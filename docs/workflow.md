# Workflow

[日本語](workflow.ja.md)

## Scope and prerequisites

The Go implementation currently obtains a Steam session ticket from a running
Le Mans Ultimate (LMU) instance and authenticates with the online backend.
Specific data retrieval, processing, and storage will be implemented separately.
No race-result endpoint or storage format is selected here.

Start LMU with the Steam account whose data you want to access and reach the main
menu. The archiver must be able to reach the LMU PC's local REST API. Set the base
URL, including host and port, with `--lmu-url`. The CLI default is
`http://127.0.0.1:6937`. Use the port the running LMU instance listens on, which
may differ from examples in the reference implementation. The same process must
also reach `https://raceos.gg` directly, without a proxy.

## 1. Obtain the LMU session ticket

```http
GET http://<Le Mans Ultimate PC>:<port>/rest/profile/getAuthSessionTicket
Accept: application/json
```

Read the `authSessionTicket` string from the JSON response. Stop if the request
fails or the ticket is missing or blank, including whitespace only; do not
attempt backend authentication.

The local API uses plain HTTP. Use it only on a trusted network and do not expose
it to the public Internet.

## 2. Authenticate with the online backend

The backend is **RaceOS (`raceos.gg`)**. This implementation sends the ticket only
to `https://raceos.gg/authenticate`, following the
[LMU Central authentication implementation](https://github.com/MarteauPiqueurdu11/LMU-Central/blob/main/lmu_central/core/raceos.py).
This is a third-party implementation reference, not an official API contract or
a runtime dependency.

```http
POST https://raceos.gg/authenticate
Accept: application/json
Content-Type: application/json

{"token":"<authSessionTicket>","game":"lmu","platform":"steam"}
```

Read the `accessToken` string from the JSON response into the Go `auth.Session`.
Stop if the token is missing or blank, including whitespace only. The client
does not assume how long a token remains valid, cache credentials on disk, or
implement automatic refresh. To obtain a new session, authenticate again while
LMU is running.

The CLI runs these two requests sequentially, waiting 1.2 seconds between them.
Each request has a 20-second timeout. It does not retry, follow redirects, or use
environment proxy settings; HTTPS certificate verification remains enabled.
HTTP errors and malformed responses stop the flow without printing response
bodies, tickets, or access tokens. Cancellation stops in-flight requests and the
wait between them.

## 3. Retrieve, process, and store data (later work)

Data retrieval is not implemented. When it is added, keep the returned session
in the same process and send its access token on later backend requests with
this header, as the reference implementation does:

```http
Game-Authorization: Bearer <accessToken>
```

The intended continuation is:

1. Retrieve the required data using the authenticated session.
2. Process it into the representation chosen for the archiver.
3. Save it to a user-controlled destination.

Endpoints, data models, transformations, storage destinations, and file formats
are deliberately undecided. The current `auth` command makes no data requests,
processes no racing data, and writes no archive. It prints only
`Authentication succeeded.` and discards the in-memory session on exit. It is a
connectivity/authentication check, not a persistent login for a later invocation.

## Run

```sh
make build
./bin/lmu-archiver auth --lmu-url http://192.0.2.10:6937
```

Replace the example address with your LMU PC's address. No third-party companion
application is required. Treat both the Steam ticket and access token as secrets:
do not log, print, or persist them. This project does not synchronize data to a
third-party dashboard or storage service.
