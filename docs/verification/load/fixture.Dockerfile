# This is ONLY the synthetic provider/private preparation image. Measured
# API/admin/worker images must be built from the normal root Dockerfile.
FROM golang:1.27.3-bookworm@sha256:21ade014faf952365b9ecbd8dbdb87176fbacb82fac9fb132d9f2a7b8cc3f132 AS build
WORKDIR /src
ENV GOMAXPROCS=2 GOFLAGS=-p=1 CGO_ENABLED=0
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/ cmd/
COPY internal/ internal/
RUN go test -c -o /out/app.test ./internal/app && \
    go build -trimpath -o /out/loadfixture ./cmd/loadfixture

FROM debian:bookworm-slim@sha256:7ff6ba31527b9e336162fa061746c366c4b2032f4d2cff2078bea43f78efe3f8
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
RUN groupadd --gid 10001 fixture && useradd --uid 10001 --gid 10001 --no-create-home fixture && \
    mkdir -p /fixture /fixture-ca /app/web /data/files && chown -R 10001:10001 /fixture /fixture-ca /app /data
COPY --from=build /out/ /app/
USER 10001:10001
WORKDIR /app
ENTRYPOINT ["/app/app.test"]
