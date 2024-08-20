# Webhook

The webhook service allows users to post files and corresponding attributes to an HTTP endpoint. It supports both CBOR and JSON body formats.

Uploaded files are saved to a local directory with the filename set as the calculated CID. Attributes are posted to the authenticated attribute service. Predefined private attributes are automatically encrypted.

## Setup

### General

Make sure the following config values are set: `AA.*`, `webhook.Host`, `Dirs.Files`, and `Dirs.EncKeys`. `Dirs.Files` is the local directory where the uploaded files are saved as `${CID}`. `Dirs.EncKeys` is where attribute encryption keys are generated/read, with filenames saved in the format of `${CID}_${ATTRIBUTE_KEY}.key`.

An environment variable `JWT_SECRET` should be set as a 32-character secret, which will be used for signing `HS256` authentication JWTs.

### Browsertrix

To use the `/browsertrix` endpoint, `Browsertrix.User` and `Browsertrix.Password` should be set as your app.browsertrix username and password. It is used to get crawl information.

`Browsertrix.WebhookSecret` is a random string for authentication that is used as querystring `s` when setting up browsertrix webhook. e.g. `Browsertrix.WebhookSecret = secret` means the webhook should be setup as `/browsertrix?s=secret`

Webhook URLs are expected to be set through app.browsertrix.com API. A JWT token can be exchanged using [login](https://app.browsertrix.com/api/redoc#tag/auth/operation/login_api_auth_jwt_login_post) and [update event webhook urls](https://app.browsertrix.com/api/redoc#tag/organizations/operation/update_event_webhook_urls_api_orgs__oid__event_webhook_urls_post). Please refer to [Browsertrix webhook setup doc](./browsertrix.md) for details.

Crawl metadata are expected to be set in `(key):(value)` format. e.g. `project_id:test_project`. `project_id` must be set, otherwise the crawl events will not be processed.

If the tag `auto-accept` is set, then the crawl will be ingested immediately after finishing. Otherwise, it will only be accepted after a manual review with a rating of Fair or above.

## Authenticating with the Webhook

For webhook callers, ensure the config values `webhook.Host` and `webhook.Jwt` are set. `webhook.Jwt` should be a pre-shared `HS256` JWT signed by the webhook host (`JWT_SECRET`).

The JWT token should be set in the `Authorization` HTTP header with a value of `Bearer: ${token}`.

## Endpoints

### POST /generic

#### Body

- **Type:** `multipart/form-data`

- **Parts:**

  | Key      | Description                                                                                    |
  | -------- | ---------------------------------------------------------------------------------------------- |
  | metadata | File metadata, attributes in key/value pairs, accepts `application/json` or `application/cbor` |
  | file     | File to be uploaded, must have file name, media type is ignored                                |

#### Description

Generic endpoint for uploading and registering a file with attributes.

#### Encryption of Private Attributes

In the `metadata` part, there is a special key called `private`. Any key-value pairs under `private` will be stored in Authenticated Attributes as attributes like normal, but encrypted, with the encryption key stored at `Dirs.EncKeys` from the config.

The encryption key is stored with the name `${CID}_${ATTRIBUTE_KEY}.key`, but CLI tools like `attr` will automatically find and use it for you.

### POST /browsertrix

#### Body

- **Type:** `application/json`

- **Parts:**
  Please refer to [Browsertrix crawlFinished event](https://app.browsertrix.com/api/redoc#operation/crawl_finishedcrawlFinished_post)

#### Description

For use with [Browsertrix cloud](https://app.browsertrix.com) webhook events. Please refer to [Browsertrix webhook setup doc](./browsertrix.md) for setup details. WACZ is downloaded and verified from crawl result. Extra metadata are fetched from the crawl's tags in the format of `(key):(value)`. Currently only supported keys are `project_id` and `asset_origin_id`.
