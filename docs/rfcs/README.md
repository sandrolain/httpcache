# RFCs referenced by this project

Plain-text copies of the RFCs this implementation relies upon, downloaded from
https://www.rfc-editor.org/ (plain-text format). Reference URLs:

| RFC | Title | Status |
|-----|-------|--------|
| RFC 9111 | HTTP Caching | Standards Track — primary spec for this library (successor of RFC 7234) |
| RFC 9110 | HTTP Semantics | Standards Track — semantics referenced by caching (headers, dates, Warning, conditional requests §13) |
| RFC 5861 | HTTP Cache-Control Extensions for Stale Content | Informational — `stale-while-revalidate`, `stale-if-error` |
| RFC 8246 | HTTP Immutable Responses | Informational — `immutable` directive |
| RFC 7231 | Hypertext Transfer Protocol (HTTP/1.1): Semantics and Content | Obsoleted by 9110/9111 — kept for legacy references in the code |
| RFC 7232 | Hypertext Transfer Protocol (HTTP/1.1): Conditional Requests | Obsoleted by 9110 — kept for legacy references in the code |

Files: `rfc<NNNN>.txt` — verbatim plain-text from rfc-editor.org.

Use these files as the authoritative reference when checking the
implementation for conformance or anomalies.
