# syntax=docker/dockerfile:1
# Imagem de produção da API (e do migrate embutido).
# Build no CI: docker build --build-arg VERSION=<sha> apps/api
FROM golang:1.25-alpine AS build
# go.mod pede 1.25.5; se a tag da imagem estiver atrás, baixa o toolchain certo.
ENV GOTOOLCHAIN=auto CGO_ENABLED=0
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG VERSION=dev
RUN go build -trimpath -ldflags "-s -w -X main.version=${VERSION}" -o /out/api ./cmd/api \
 && go build -trimpath -ldflags "-s -w" -o /out/migrate ./cmd/migrate

# Runtime alpine (não distroless): wget do busybox para healthcheck do compose,
# shell para diagnóstico em incidente, ca-certificates para TLS (Neon/DAV).
# tzdata já vem embutido no binário (import de time/tzdata em cmd/api).
FROM alpine:3.21
RUN adduser -D -H -u 10001 app
COPY --from=build /out/api /out/migrate /
USER app
EXPOSE 8090
# CMD (não ENTRYPOINT): o deploy roda migrations com `docker run ... /migrate up`.
CMD ["/api"]
