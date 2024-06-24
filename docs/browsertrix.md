# Browsertrix Webhook setup

Guide to setup app.browsertrix.com for receiving webhook events

## Authentication

Currently the API only support using username and password to exchange a JWT token with ~3 months expiration. Please refer to `Login` session below.

After getting the JWT, set `Authorization: Bearer ${JWT}` when accessing authenticated routes

## Login

POST `https://app.browsertrix.com//api/auth/jwt/login`

Use your account username and password to exchange for JWT

```
curl --location 'https://app.browsertrix.com//api/auth/jwt/login' \
--header 'Content-Type: application/x-www-form-urlencoded' \
--header 'Accept: application/json' \
--data-urlencode 'username=<string>' \
--data-urlencode 'password=<string>'
```

Response:

```
{
"access_token": "string",
"token_type": "string"
}
```

## Get Organization ID

GET `https://app.browsertrix.com/api/orgs`

List organization to receive the organization ID needed for API calls

```
curl --location 'https://app.browsertrix.com/api/orgs' \
--header 'Authorization: Bearer <jwt>
```

Response:

```
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

```
curl --location 'https://app.browsertrix.com/api/orgs/<org-id>/event-webhook-urls' \
--header 'Content-Type: application/json' \
--header 'Authorization: Bearer <jwt>
--data '{
  "crawlStarted": "<webhook-url>",
  "crawlFinished": "<webhook-url>",
  "crawlDeleted": "<webhook-url>",
  "uploadFinished": "<webhook-url>",
  "uploadDeleted": "<webhook-url>",
  "addedToCollection": "<webhook-url>",
  "removedFromCollection": "<webhook-url>",
  "collectionDeleted": "<webhook-url>"
}'
```

Response:

```
{
    "updated": true
}
```
