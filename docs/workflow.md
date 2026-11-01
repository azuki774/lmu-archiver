# Workflow

[日本語](workflow.ja.md)

1. **Obtain a session ticket from LMU.**
   Get a ticket for authentication from the running Le Mans Ultimate instance.
2. **Authenticate with raceos.gg.**
   Use the ticket to obtain credentials for accessing racing data.
3. **Retrieve, process, and store data.**
   Use those credentials to retrieve the required data, process it for archiving,
   and save it to a user-controlled destination.

The current implementation covers ticket acquisition and authentication.
Specific data retrieval, processing, and storage will be implemented separately.

## API request pacing

The CLI shares one HTTP client for LMU and RaceOS. Each transport attempt is
followed by at least a one-second cooldown after response headers or a connection
error, shared by concurrent callers. Waiting is cancelable and counts toward the
20-second request timeout. The existing 1.2-second authentication wait remains;
requests are not automatically retried. Separate processes, transport-internal
retries, and server arrival timing are outside this guarantee.

