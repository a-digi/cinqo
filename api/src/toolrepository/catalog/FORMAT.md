# Tool repository file, format version 1

A repository is one JSON file, registered by its own link (any file name). Unknown
fields are ignored. A file that breaks any rule is rejected whole, with every problem listed.

```json
{
  "formatVersion": 1,
  "name": "My tools",
  "description": "Tools published by me",
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
| `formatVersion` | must be `1` |
| `name`, `description` | optional; at most 100 / 2,000 characters |
| `tools` | at most 200; `id` unique, `^[a-z][a-z0-9_]*$` (the tool's manifest slug) |
| `currentVersionId` | required; must equal a `versionId` of the same tool |
| `versions` | 1–200 per tool; `versionId` unique per tool, 1–100 of `[A-Za-z0-9._-]`; `version` is `MAJOR.MINOR.PATCH`, unique per tool |
| `name` | required, at most 100 characters |
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
