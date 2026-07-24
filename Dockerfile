# syntax=docker/dockerfile:1

FROM node:22-alpine AS frontend-builder
WORKDIR /src/frontend
COPY frontend/package.json frontend/package-lock.json ./
RUN npm ci
COPY frontend/ ./
RUN npm run build

FROM golang:1.26-alpine AS backend-builder
WORKDIR /src/backend
COPY backend/go.mod backend/go.sum ./
RUN go mod download
COPY backend/ ./
COPY --from=frontend-builder /src/frontend/dist ./internal/webui/dist
# CGO_ENABLED=0 works because the sqlite driver (glebarez/sqlite) is pure Go
# — a deliberate choice made for exactly this: no musl/gcc toolchain needed.
RUN CGO_ENABLED=0 go build -o /out/ocpp-station-simulator ./cmd/server

FROM alpine:3.20
RUN adduser -D -u 10001 appuser && mkdir -p /data && chown appuser:appuser /data
WORKDIR /app
COPY --from=backend-builder /out/ocpp-station-simulator ./ocpp-station-simulator
USER appuser
ENV PORT=8080 DB_DRIVER=sqlite DB_DSN=/data/ocpp-simulator.db
EXPOSE 8080
VOLUME ["/data"]
ENTRYPOINT ["./ocpp-station-simulator"]
