# Third-party notices — Browser tool

The Browser tool's Katana fetch engine (plan 184) links these libraries into the
tool's binary. They are used unmodified; their source is available from the
repositories named below, at the versions in `backend/go.mod`.

| Library | Version | Licence | Source |
|---|---|---|---|
| Katana (`github.com/projectdiscovery/katana`) | v1.8.0 | MIT | https://github.com/projectdiscovery/katana |
| `github.com/projectdiscovery/retryablehttp-go` | v1.3.29 | MPL-2.0 | https://github.com/projectdiscovery/retryablehttp-go |
| `github.com/hashicorp/go-version` | v1.8.0 | MPL-2.0 | https://github.com/hashicorp/go-version |
| `github.com/hashicorp/golang-lru/v2` | v2.0.7 | MPL-2.0 | https://github.com/hashicorp/golang-lru |
| `github.com/gosimple/slug` | v1.15.0 | MPL-2.0 | https://github.com/gosimple/slug |

MPL-2.0 is file-level: it applies to those libraries' own files, which are not
changed here. The full licence text ships with each module's source
(`LICENSE`), and is at https://www.mozilla.org/MPL/2.0/.

The other dependencies of Katana (about 125 modules) carry their own licences,
listed in each module's `LICENSE` file; none of them was found to be a
copyleft licence stronger than MPL-2.0 when this notice was written.
