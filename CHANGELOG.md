# Changelog

## v0.1.0 (2026-11-19)

Vista shows everything Sonar and Blackbox now serve, and the Overview is laid out as a server dashboard.

### Added

- The Overview has tiles for Spark's health and uptime, containers running, CPU, memory, disk and network with their trends; a CPU, memory and disk chart with deploys marked; the five containers using the most CPU, memory or network; server information, disk usage and network traffic; every container with its state, use against its limits, restarts and uptime; the load average and each core's use; and error counts per project with repeated errors grouped.
- Spark's state is Sonar's own, from its conditions, and each condition, such as a filling disk, is listed under what needs attention.
- The Spark page shows swap, load, disk and network over time, which of Sonar's measurements work, sparkdb's connections and database sizes, disk use per project, and traffic per site from Cloudflare.
- Each project shows since when it has been in its state, its outside checks, its containers, its CPU and memory over time, its data folder and databases, its traffic, its error counts, and the counters and checks the app itself reports.
- Activity lists each time a project was down or degraded, with the services and checks that weren't up, and charts error and warning lines per hour.
- Incidents lists every stretch a project was down or degraded in the last 30 days, with when it began and ended.
- The Projects table shows each project's error lines in the chosen range.
- A search box at the top of every page searches the logs.

### Changed

- Host measurements come from Sonar's detailed host answer instead of its public history.
- A project is shown as infrastructure when Sonar's group label says so; the built-in list covers projects without the label.
- The pages use line icons, and the Sources page no longer lists measurements as not served.

## v0.0.0 (2026-11-19)

The first version: a read-only dashboard over Sonar, Blackbox and Lighthouse, behind Cloudflare Access.

### Added

- The Overview shows what needs attention, project states, host CPU and memory with deploy markers, recent deploys, the newest error lines and log storage.
- The Projects page lists every project with its state, version, monitoring method, response time, uptime with coverage and 30-day history, and opens each project's details, recent errors and links to its logs and activity.
- The Spark page shows the server apart from the projects: its measurements, the infrastructure projects' states, log collection, and which measurements aren't collected yet.
- The Activity page puts deploys and log gaps on one timeline, filtered by project and kind, and opens each deploy's steps and the output of the step that failed.
- Deploys come from Lighthouse's records when Lighthouse is connected, and from the container swaps Sonar saw when it isn't. Each project shows its deploy state, version and releases, and a project whose deploys keep failing needs attention.
- The Logs page searches Blackbox by text, project, level, version and time, shows a line's fields and the lines around it, and links to any line.
- The Incidents page lists availability and the days each project was down or degraded.
- The Sources page shows where each piece of information comes from and how fresh it is.
- Vista shows only what its sources report, asking them only while a page is open. A source that stops answering keeps its last answer on screen marked stale, and live states become unknown instead of healthy.
- Every request except the health check needs a valid Cloudflare Access token; in production Vista won't start without the Access settings.
