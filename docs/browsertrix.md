# Browsertrix Webhook setup

This is a guide to set up app.browsertrix.com so it sends webhook events to the Integrity v2 server.
This allows Browsertrix crawls to be ingested into the data pipeline automatically.

## Authentication

Currently the API only support using username and password to exchange a JWT token with ~3 months expiration. Please refer to `Login` session below.

After getting the JWT, set `Authorization: Bearer ${JWT}` when accessing authenticated routes

## Login

POST `https://app.browsertrix.com/api/auth/jwt/login`

Use your account username and password to exchange for JWT

```bash
curl --location 'https://app.browsertrix.com/api/auth/jwt/login' \
--header 'Content-Type: application/x-www-form-urlencoded' \
--header 'Accept: application/json' \
--data-urlencode 'username=<string>' \
--data-urlencode 'password=<string>'
```

Response:

```json
{
  "access_token": "string",
  "token_type": "string"
}
```

## Get Organization ID

GET `https://app.browsertrix.com/api/orgs`

List organization to receive the organization ID needed for API calls

```bash
curl --location 'https://app.browsertrix.com/api/orgs' \
--header 'Authorization: Bearer <jwt>
```

Response:

```json
{
    "items": [
        {
            "id": "cb8515f9-7622-4879-b79e-d1f084a11ea2",
            "name": "Starling Lab",
            "slug": "starling-lab",
            "users": {
              ...
            }
            ...
        }
    ]
}
```

## Set Webhook URL Config

POST `https://app.browsertrix.com/api/orgs/<org-id>/event-webhook-urls`

Set the set of webhook URLs to be used for all crawls in an organization

```bash
curl --location 'https://app.browsertrix.com/api/orgs/<org-id>/event-webhook-urls' \
--header 'Content-Type: application/json' \
--header 'Authorization: Bearer <jwt>' \
--data '{
  "crawlStarted": null,
  "crawlFinished": "<webhook url>",
  "crawlDeleted": null,
  "uploadFinished": null,
  "uploadDeleted": null,
  "addedToCollection": null,
  "removedFromCollection": null,
  "collectionDeleted": null,
  "qaAnalysisFinished" : null,
  "crawlReviewed": "<webhook url>"
}'
```

Response:

```json
{
  "updated": true
}
```

The `<webhook url>` might look something like this: `https://example.com/browsertrix?s=abc123`. The path is always `/browsertrix`. The query param `s` is a secret value to prevent others from submitting crawl data. It must match the `Browsertrix.WebhookSecret` value in the config. See [webhook.md](./webhook.md) for more details.


## Done

Now the server will be notified when a crawl finishes or is reviewed, and can act accordingly. See [webhook.md](./webhook.md) for more details on how this is used.
