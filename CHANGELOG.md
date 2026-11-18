# Changelog

## v0.0.0 (2026-11-18)

The first version: a read-only dashboard over Sonar and Blackbox, behind Cloudflare Access.

### Added

- The Overview shows what needs attention, project states, host CPU and memory with deploy markers, recent deploys, the newest error lines and log storage.
- The Projects page lists every project with its state, version, monitoring method, response time, uptime with coverage and 30-day history, and opens each project's details, recent errors and links to its logs and activity.
- The Spark page shows the server apart from the projects: its measurements, the infrastructure projects' states, log collection, and which measurements aren't collected yet.
- The Activity page puts deploys and log gaps on one timeline, filtered by project and kind.
- The Logs page searches Blackbox by text, project, level, version and time, shows a line's fields and the lines around it, and links to any line.
- The Incidents page lists availability and the days each project was down or degraded.
- The Sources page shows where each piece of information comes from and how fresh it is.
- Vista shows only what Sonar and Blackbox report, asking them only while a page is open. A source that stops answering keeps its last answer on screen marked stale, and live states become unknown instead of healthy.
- Every request except the health check needs a valid Cloudflare Access token; in production Vista won't start without the Access settings.
