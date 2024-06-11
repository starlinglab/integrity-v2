# Webhook

The webhook service allows users to post files and corresponding attributes to an HTTP endpoint. It supports both CBOR and JSON body formats.

Uploaded files are saved to a local directory with the filename set as the calculated CID. Attributes are posted to the authenticated attribute service. Predefined private attributes are automatically encrypted.

## Setup

Make sure the following config values are set: `AA.*`, `webhook.Host`, `Dirs.Files`, and `Dirs.EncKeys`. `Dirs.Files` is the local directory where the uploaded files are saved as `${CID}`. `Dirs.EncKeys` is where attribute encryption keys are generated/read, with filenames saved in the format of `${CID}_${ATTRIBUTE_KEY}.key`.

An environment variable `JWT_SECRET` should be set as a 32-character secret, which will be used for signing `HS256` authentication JWTs.

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

## Encryption of Private Attributes

When the attribute key matches the list of predefined private attributes (currently hardcoded values are "private" and "proofmode"), the webhook automatically encrypts the attribute key before registering it on the authenticated attribute service.
Private attributes can be either be a simple value, or a nested map of key value pairs. In the case of a map, keys are not encrypted.
A 32-byte private key is automatically read or generated from `Dirs.EncKeys`, in the format of `${CID}_${ATTRIBUTE_KEY}.key`.
