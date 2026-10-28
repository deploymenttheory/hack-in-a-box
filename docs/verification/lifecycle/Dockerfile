# syntax=docker/dockerfile:1
# Explicit verification image: this contains Go _test.go fixture controls and
# must never be published or described as the production application image.
FROM node:22.23.3-bookworm-slim@sha256:3ed98443660ff48413b5af19ecefce3b9d50845a901a43420d47cc63a9fddbe9 AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
ENV VITE_STYLEGUIDE_URL=http://styleguide.localhost:8080
RUN npm run build

FROM golang:1.27.3-bookworm@sha256:21ade014faf952365b9ecbd8dbdb87176fbacb82fac9fb132d9f2a7b8cc3f132 AS build
WORKDIR /src
ENV GOMAXPROCS=2 GOFLAGS=-p=2 CGO_ENABLED=0
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/ cmd/
COPY internal/ internal/
RUN --mount=type=cache,id=hackbox-lifecycle-go-build,target=/root/.cache/go-build go test -c -trimpath -o /out/lifecycle.test ./internal/app && \
    go build -trimpath -o /out/migrate ./cmd/migrate && \
    go build -trimpath -o /out/healthcheck ./cmd/healthcheck

FROM debian:bookworm-slim@sha256:7ff6ba31527b9e336162fa061746c366c4b2032f4d2cff2078bea43f78efe3f8
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
RUN groupadd --gid 10001 lifecycle && useradd --uid 10001 --gid 10001 --no-create-home lifecycle && \
    mkdir -p /control /data/files /artifacts /app && \
    chown -R 10001:10001 /control /data /artifacts /app && chmod 0700 /control /artifacts
COPY --from=build /out/ /app/
COPY --from=build /usr/local/go/lib/time/zoneinfo.zip /app/zoneinfo.zip
COPY --from=web /src/web/dist/ /app/web-public/
COPY --from=web /src/web/dist-admin/ /app/web-admin/
USER 10001:10001
WORKDIR /app
ENV ZONEINFO=/app/zoneinfo.zip
ENTRYPOINT ["/app/lifecycle.test"]
