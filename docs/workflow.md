# Workflow

[日本語](workflow.ja.md)

1. **Obtain a session ticket from LMU.**
   Get a ticket for authentication from the running Le Mans Ultimate instance.
2. **Authenticate with raceos.gg.**
   Use the ticket to obtain credentials for accessing racing data.
3. **Retrieve, process, and store data.**
   Use those credentials to retrieve the required data, process it for archiving,
   and save it to a user-controlled destination.

The CLI implements ticket acquisition, authentication, current rating observation,
and SQLite import/query of existing results JSON. Raw results stay in separate
files; the database stores the tracked player's normalized data. Reference
ratings are selected on read from observations at or before the event timestamp.
Automated results API retrieval is not implemented.
