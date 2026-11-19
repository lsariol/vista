# The Go version matches go.mod's; they're bumped together.
FROM golang:1.27.4-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o vista ./cmd/vista

# Built before every deploy: a failure stops the deploy.
FROM builder AS test
RUN apk add --no-cache gcc musl-dev
RUN unformatted="$(gofmt -l .)" && [ -z "$unformatted" ] || { echo "These files need gofmt:"; echo "$unformatted"; exit 1; }
RUN go vet ./...
RUN CGO_ENABLED=1 go test -race -count=1 ./...

# Pinned on purpose; bumped with a release.
FROM alpine:3.24
COPY --from=builder /app/vista /vista
USER 10001:10001
EXPOSE 3010
CMD ["/vista", "serve"]
