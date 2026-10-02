# The panel against a demo agent, to try a change before it is released.
#
#   docker compose -f dev/compose.yaml up --build
#
# then http://localhost:8080, user "demo", password "demo-password".
# Nothing runs on any host: the agent answers every command itself.

FROM node:22-alpine AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json ./
RUN npm ci --no-audit --no-fund
COPY web/ ./
RUN npm run build

FROM golang:1.27-alpine AS croft
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY cmd/ cmd/
COPY internal/ internal/
COPY --from=web /src/internal/server/dist/ internal/server/dist/
RUN CGO_ENABLED=0 go build -ldflags "-X main.version=dev" -o /croft ./cmd/croft

FROM alpine:3.22
COPY --from=croft /croft /usr/local/bin/croft
COPY dev/entrypoint.sh /entrypoint.sh
EXPOSE 8080
ENTRYPOINT ["/bin/sh", "/entrypoint.sh"]
