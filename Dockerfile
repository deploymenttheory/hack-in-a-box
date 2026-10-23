# syntax=docker/dockerfile:1
FROM node:22.23.3-bookworm-slim@sha256:3ed98443660ff48413b5af19ecefce3b9d50845a901a43420d47cc63a9fddbe9 AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

FROM golang:1.27.3-bookworm@sha256:21ade014faf952365b9ecbd8dbdb87176fbacb82fac9fb132d9f2a7b8cc3f132 AS go
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/ cmd/
COPY internal/ internal/
RUN CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /out/api ./cmd/api && \
    CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /out/worker ./cmd/worker && \
    CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /out/migrate ./cmd/migrate && \
    CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /out/healthcheck ./cmd/healthcheck && \
    CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /out/styleguide ./cmd/styleguide

FROM debian:bookworm-slim@sha256:7ff6ba31527b9e336162fa061746c366c4b2032f4d2cff2078bea43f78efe3f8 AS runtime
COPY --from=go /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
RUN groupadd --gid 10001 hackbox && useradd --uid 10001 --gid 10001 --no-create-home hackbox && \
    mkdir -p /data/files /app/web && chown -R 10001:10001 /data /app
COPY --from=go /out/ /app/
USER 10001:10001
WORKDIR /app
ENV HACKBOX_HTTP_ADDR=0.0.0.0:8080 HACKBOX_STORAGE_PATH=/data/files HACKBOX_WEB_DIR=/app/web
EXPOSE 8080
ENTRYPOINT ["/app/api"]

FROM runtime AS admin
COPY --from=web /src/web/dist-admin/ /app/web/
ENV HACKBOX_SURFACE=admin

FROM runtime AS public
COPY --from=web /src/web/dist/ /app/web/
ENV HACKBOX_SURFACE=public


FROM web AS styleguide-web
ARG STYLEGUIDE_REVISION=local
ENV VITE_STYLEGUIDE_REVISION=$STYLEGUIDE_REVISION
RUN npm run build:styleguide

FROM runtime AS styleguide
COPY --from=styleguide-web /src/web/dist-styleguide/ /app/web/
ENTRYPOINT ["/app/styleguide"]
