# Folder Preprocessor

The Folder Preprocessor recursively monitors file changes within a directory and submits the file and its metadata to a webhook if the file meets specific criteria.

This preprocessor must be run with a PostgreSQL database. Users are expected to set up Project Metadata in the admin database before starting to sync the folders.

## Setup

### Config

The following is the required configuration in TOML.

#### FolderPreprocessor

`SyncFolderRoot` must be set to define which directory should be watched for file changes.

#### FolderDatabase

Credentials to connect to a PostgreSQL Database. The `Database` must be created beforehand.

### Webhook

`Host` should be set to where a webhook service is hosted. `Jwt` should be a pre-shared secret for authentication to the webhook.

### Database

#### Project Metadata (`project_metadata`)

This table allows users to define `project_id` and corresponding `project_path` file path (relative to the sync root directory) to watch for new files. `file_extensions` also allow users to only watch for specific file extensions.

If a file does not belong to any project or is not in `file_extensions` (if set), it is not handled by the folder preprocessor.

| Column            | Description                                                                                                                      |
| ----------------- | -------------------------------------------------------------------------------------------------------------------------------- |
| project_id        | A unique identifier of the project, preferably human-readable.                                                                   |
| project_path      | The path where files are located relative to the sync folder root, e.g., `/project-id`.                                          |
| author_type       | (optional) Author type according to [Schema.org/author](https://schema.org/author).                                              |
| author_name       | (optional) Author name according to [Schema.org/author](https://schema.org/author).                                              |
| author_identifier | (optional) Author identifier according to [Schema.org/author](https://schema.org/author).                                        |
| file_extensions   | (optional) A list of allowed file extensions to watch for. If not set, all file extensions are watched. e.g., `.gif,.jpg,.jpeg`. |

#### File Status

This table stores all files that are found and match the criteria.

| Column     | Description                                                                                                                   |
| ---------- | ----------------------------------------------------------------------------------------------------------------------------- |
| id         | Auto-increment unique ID, not used by humans.                                                                                 |
| file_path  | The file path where this file is found, unique key.                                                                           |
| status     | Status of the file, can be one of `Found`, `Uploading`, `Success`, `Error`.                                                   |
| error      | If the status is `Error`, the error message is stored here.                                                                   |
| cid        | The resulting content ID if the status is `Success`. Only the first CID is recorded if the file induces more than one upload. |
| created_at | The time the file is first found.                                                                                             |
| updated_at | The time of the last status update.                                                                                           |

#### Allowed keys

This table contains public keys that are accepted for verification by the preprocessor. This table must be filled out, otherwise no signed content will be verified. Unsigned content such as regular files are always accepted however.

| Column     | Description                                   |
| ---------- | --------------------------------------------- |
| id         | Auto-increment unique ID, not used by humans. |
| project_id | Project ID the key applies to                 |
| key_type   | The kind of key this is: `proofmode`, `wacz`  |
| key        | Actual text of the key                        |
| name       | Name for the key, like "John Doe's iPhone"    |

For `proofmode`, the value of `key` must be the fingerprint of the user's PGP public signing key, such as `6C7EE1B8621CC013` or `0xdf414b25e818cd46`. Note how case doesn't matter, and optionally a prefix of `0x` is accepted.

For `wacz`, the value of `key` must be the public key of the WACZ anonymous signer, which is PEM-encoded. This can be found within the `.wacz` ZIP, under `datapackage-digest.json`, under `signedData.publicKey`. An example public key would look like:

```
MHYwEAYHKoZIzj0CAQYFK4EEACIDYgAEdyJB4zGpvCzhNblldx8b12sz+ECGk8Ryq4y+bg9woRu3OSKWO2uS+n8CD258iVvg0hP0JRg4C7YxGc7lqGsI9bHj0NaC9b4NXazeuR80iVCg96oTYIOLdWcII9rfaFMU
```

## Usage

When the preprocessor is first launched, it will scan all `project_paths` under the sync root folder. Any files found and matching the handling criteria will be processed.

After that, the preprocessor recursively watches for changes under the same paths and handles any new or renamed files according to the criteria.

The preprocessor keeps track of discovered file statuses in the database. Any file (identified by file path) that has a status other than `Found` will not be re-processed. The CID is set for the `Success` status and the `error` is set for the `Error` status.

## Ignored Files (Criteria for file to be processed)

For a file to be processed by this preprocessor, it must fit certain criteria:

- It must be under the `project_path` of a project, i.e. it must belong to a project.
- If `file_extensions` is set in the project, it must have an allowed extension.
- The file name must not begin with `.` (i.e. no hidden files).
- The file name must not end with `.partial`.
- The file status must not be `Error` or `Complete`.

## Special File Types

### ProofMode

This preprocessor parses `ProofMode` bundles. If a `.zip` file contains a `HowToVerifyProofData.txt` at the root, it is treated as a `ProofMode` bundle. All signatures and hashes will be verified.

If a `ProofMode` bundle contains more than one asset, all assets will be verified and individually uploaded to the webhook. Additionally, a special attribute `proofmode` will be set to contain signature-related information.

### WACZ

This preprocessor parses `wacz` bundles. If a zip file with the `.wacz` extension contains a `datapackage.json` at the root, it is treated as a `wacz` bundle.

This preprocessor supports verifying both anonymously signed and domain-signed `wacz` bundles. For anonymous bundles, we don't have a trusted list of keys, but a public key whitelist will be introduced soon. For domain signed bundle, a hardcode list of Let's Encrypt root cert, and freetsa root cert for RFC3161 verification is whitelisted.

Additionally, a special attribute `wacz` would be set to contain signature-related information.
