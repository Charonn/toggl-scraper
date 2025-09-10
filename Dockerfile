# Multi-stage build
FROM golang:1.25 AS builder
WORKDIR /src

# Leverage module caching
COPY go.mod ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o /out/toggl-scraper ./cmd/toggl-scraper

FROM gcr.io/distroless/static:nonroot
WORKDIR /
COPY --from=builder /out/toggl-scraper /toggl-scraper
USER nonroot:nonroot

# Default to daily-at-midnight mode in UTC; override SYNC_TZ to change timezone
ENV SYNC_TZ=UTC
EXPOSE 8085
ENTRYPOINT ["/toggl-scraper","--daily"]
