# syntax=docker/dockerfile:1

# ── build ────────────────────────────────────────────────────────────────
FROM golang:1.25-alpine AS build
WORKDIR /src
# Cache module downloads independently of source changes.
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# Static binaries (no CGO) so they run on distroless/scratch.
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/server
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/seed ./cmd/seed

# ── seeder (one-off: wipes the DB and loads demo data) ──────────────────
# Built only when requested via `target: seed` (compose `seed` service).
FROM gcr.io/distroless/static-debian12:nonroot AS seed
COPY --from=build /out/seed /seed
USER nonroot:nonroot
ENTRYPOINT ["/seed"]

# ── runtime (default target: the API) ───────────────────────────────────
FROM gcr.io/distroless/static-debian12:nonroot AS api
COPY --from=build /out/server /server
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/server"]
