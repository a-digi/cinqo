# Tool repository file, format version 2

A repository is one JSON file, registered by its own link (any file name). Unknown
fields are ignored. A file that breaks any rule is rejected whole, with every problem listed.

**Who made it is obligatory from format version 2:** the repository's `authors`, and each tool
version's `authors` and `license`. A **version 1** file (no authors) is still read; the app
shows its developers as "not stated". Everything below that is about authors applies to both
the repository file and a tool's `manifest.json`, which carries the same tool fields; the
rules live once, in `api/src/toolmeta`.

```json
{
  "formatVersion": 2,
  "name": "My tools",
  "description": "Tools published by me",
  "authors": [{ "name": "Ada Lovelace", "email": "ada@example.com", "url": "https://ada.example" }],
  "company": { "name": "Acme GmbH", "url": "https://acme.example" },
  "links": { "homepage": "https://acme.example", "security": "mailto:security@acme.example" },
  "generatedAt": "2026-10-31T10:00:00Z",
  "tools": [
    {
      "id": "browser",
      "currentVersionId": "v2",
      "versions": [
        {
          "versionId": "v2",
          "name": "Browser",
          "description": "Headless browser",
          "version": "0.0.190",
          "authors": [{ "name": "Ada Lovelace", "email": "ada@example.com" }],
          "company": { "name": "Acme GmbH" },
          "license": "MIT",
          "links": { "homepage": "https://…", "source": "https://…", "issues": "https://…" },
          "copyright": "© 2026 Acme GmbH",
          "category": "browser",
          "keywords": ["crawl", "chrome"],
          "releasedAt": "2026-10-30",
          "changelog": "…",
          "minAppVersion": "0.0.100",
          "packages": [
            { "os": "darwin", "arch": "arm64", "url": "files/tool-browser-darwin-arm64.zip",
              "sha256": "<64 hex>", "sizeBytes": 1234567 }
          ]
        }
      ]
    }
  ]
}
```

| Field | Rule |
| --- | --- |
| `formatVersion` | `2` (or `1`, the older format without authors) |
| `authors` | **required in format 2** (1–20); each `{name, email?, url?}`: `name` required, 1–100 characters, no control or text-direction characters; `email` `name@example.com` shaped, at most 254; `url` `https` only |
| `company` | optional `{name, url?}`; `name` required, at most 100 |
| `links` | optional; `homepage`, `documentation`, `source`, `issues`, `support` and `security`, each an `https` address (`security` may also be `mailto:name@example.com`), at most 2,000 characters |
| `generatedAt` | optional; a date or an RFC 3339 timestamp, set by `repotool` |
| `name`, `description` | optional; at most 100 / 2,000 characters |
| `tools` | at most 200; `id` unique, `^[a-z][a-z0-9_]*$` (the tool's manifest slug) |
| `currentVersionId` | required; must equal a `versionId` of the same tool |
| `versions` | 1–200 per tool; `versionId` unique per tool, 1–100 of `[A-Za-z0-9._-]`; `version` is `MAJOR.MINOR.PATCH`, unique per tool |
| `name` | required, at most 100 characters |
| `authors`, `license` (per version) | **required in format 2**; `authors` as above; `license` an SPDX identifier or expression (`MIT`, `Apache-2.0`, `MIT OR Apache-2.0`) or `Proprietary`, at most 100 |
| `company`, `links`, `copyright`, `category`, `keywords` (per version) | optional; `company` and `links` as above; `copyright` at most 200; `category` one of `productivity`, `data`, `ai`, `browser`, `documents`, `developer`, `other`; `keywords` at most 10, each 1–30 letters, digits, spaces or dashes |
| `description`, `changelog` | optional; at most 2,000 / 5,000 characters |
| `releasedAt` | optional; `2026-10-30` or an RFC 3339 timestamp |
| `minAppVersion` | optional; `MAJOR.MINOR.PATCH` |
| `packages` | 1–20 per version; one per `os`/`arch` |
| `os` / `arch` | `darwin`, `linux`, `windows`, `any` / `arm64`, `amd64`, `any` |
| `url` | absolute, or relative to the repository file's own address; `https`, or `http` for a local address only; no credentials; at most 2,000 characters |
| `sha256` | 64 hexadecimal characters (stored lower-case) |
| `sizeBytes` | optional; 0 to 200 MB |

The file itself is at most 2 MB. Package choice for a machine: exact `os`/`arch`, then
`os`/`any`, then `any`/`arch`, then `any`/`any`.

An address in any of these fields is only ever **shown as a link**: the app never fetches it, and `http`, `javascript:` and `data:` addresses are rejected. Names and addresses are a **claim by the publisher**, not proof of identity.
