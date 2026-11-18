# Vista

Vista is a private, read-only dashboard for Spark, the server Mobasity's projects run on, and for the projects themselves.

## Features

- See what needs attention first: projects that are down or degraded, failed deploys, log gaps and sources that stopped answering.
- Follow Spark apart from the projects: its CPU and memory, and the state of the services every project relies on.
- Check every project's state, version, monitoring method, response time, 30- and 90-day uptime with coverage, and daily history.
- Read deploys and log gaps on one timeline, filtered by project.
- See the newest error lines across every project.
- Search the logs every container wrote, open a line's fields, and read the lines around it.
- Review availability and the days each project was down or degraded.
- See where each piece of information comes from and how fresh it is. A source that can't be read is shown as unknown, never as zero or healthy.

Vista collects and judges nothing itself: it shows what other services report. It reads [Sonar](https://github.com/lsariol/sonar) for states, uptime, host measurements and deploys, and [Blackbox](https://github.com/lsariol/blackbox) for logs, through their read-only APIs, and only while a page is open. It has no controls: it can't deploy, restart, change settings or read secrets.

## Requirements

- Go 1.27.4
- Docker, to run the test stage or the container

## Getting started

Vista needs data to show. For a local run, `scripts/devsources` stands in for Sonar and Blackbox with made-up data. Start it in one terminal:

```bash
go run ./scripts/devsources
```

Then start Vista in another:

```bash
APP_ENV=dev \
VISTA_SONAR_URL=http://localhost:2599 VISTA_SONAR_TOKEN=dev \
VISTA_BLACKBOX_URL=http://localhost:2599 VISTA_BLACKBOX_TOKEN=dev \
go run ./cmd/vista serve
```

Open <http://localhost:3000>. `go run ./cmd/vista health` exits 0 while it's up, and `go run ./cmd/vista help` lists the other commands.

With `APP_ENV=dev` and no Cloudflare Access settings, Vista lets every request through and logs a warning. With `APP_ENV=prod` it refuses to start without them.

## Configuration

Everything is read from the environment.

| Variable | Default | Meaning |
|---|---|---|
| `APP_ENV` | `prod` | `prod`, or `dev` for readable logs and an optional sign-in check |
| `VISTA_ADDR` | `:3000` | Address to listen on |
| `VISTA_VERSION` | `dev` | Version it reports |
| `LOG_LEVEL` | `info` | `debug`, `info`, `warn` or `error` |
| `VISTA_ACCESS_TEAM` | | Cloudflare Access team name, the part before `.cloudflareaccess.com` |
| `VISTA_ACCESS_AUD` | | The Access application's audience tag |
| `VISTA_SONAR_URL` | | Sonar's API, such as `http://sonar-api:2500` |
| `VISTA_SONAR_TOKEN` | | Vista's bearer token for Sonar |
| `VISTA_BLACKBOX_URL` | | Blackbox's API, such as `http://blackbox-api:3190` |
| `VISTA_BLACKBOX_TOKEN` | | Vista's bearer token for Blackbox |

A source without its address and token is shown as not connected; Vista still starts.

### Signing in

Vista is meant to sit behind [Cloudflare Access](https://developers.cloudflare.com/cloudflare-one/applications/configure-apps/self-hosted-public-app/). Access signs the visitor in and forwards each request with a signed token. Vista checks that token's signature against the team's published keys, its issuer, its audience tag and its expiry on every request except `/healthz`, so a request that reaches it any other way is refused.

## Development

```bash
go test -race ./...
docker build --target test .   # gofmt, vet and the tests, as before a deploy
```

The dashboard is plain HTML, CSS and JavaScript in `internal/web/static`, built into the binary; there is no build step. Every value from a source is put on the page as text, never as HTML, because log lines are untrusted.

| Folder | What's in it |
|---|---|
| `cmd/vista` | The binary: `serve`, `health`, `version`, `help` |
| `internal/access` | The Cloudflare Access check |
| `internal/config` | Settings, read once and checked |
| `internal/source` | Sonar and Blackbox clients, and the cache that keeps a source's last answer |
| `internal/dashboard` | The sources' answers arranged for each page |
| `internal/web` | The HTTP server, Vista's JSON API and the pages |
| `scripts/devsources` | Made-up sources for local runs |

## Deployment

`docker-compose.yml` is written for [Lighthouse](https://github.com/lsariol/lighthouse), which deploys each version tag. It expects an external Docker network named `spark`, on which Sonar and Blackbox answer, and reads its tokens and Access settings from the environment. Changes are listed in [CHANGELOG.md](CHANGELOG.md).

## License

See [LICENSE](LICENSE).
