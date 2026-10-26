# syntax=docker/dockerfile:1
FROM golang:1.27.3-bookworm@sha256:21ade014faf952365b9ecbd8dbdb87176fbacb82fac9fb132d9f2a7b8cc3f132 AS build
WORKDIR /source
ENV GOMAXPROCS=2 GOFLAGS=-p=1
COPY main.go ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /out/tlsproxy main.go

FROM debian:bookworm-slim@sha256:7ff6ba31527b9e336162fa061746c366c4b2032f4d2cff2078bea43f78efe3f8
RUN groupadd --gid 10001 fixture && useradd --uid 10001 --gid 10001 --no-create-home fixture
COPY --from=build /out/tlsproxy /app/tlsproxy
USER 10001:10001
ENV GOMAXPROCS=2
EXPOSE 8443 8444 8445
ENTRYPOINT ["/app/tlsproxy"]
