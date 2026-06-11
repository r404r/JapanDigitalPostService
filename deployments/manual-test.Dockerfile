FROM node:22-alpine AS web-build
WORKDIR /src/web
COPY web/package*.json ./
RUN npm ci
COPY web/ ./
RUN npm run build

FROM golang:1.22-alpine AS go-build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/server

FROM alpine:3.20
RUN apk add --no-cache ca-certificates && adduser -D -H -u 10001 appuser
WORKDIR /app
COPY --from=go-build /out/server /app/server
COPY --from=web-build /src/web/dist /app/web
RUN mkdir -p /data && chown -R appuser:appuser /data
USER appuser
ENV HTTP_ADDR=:8080
ENV STATIC_DIR=/app/web
EXPOSE 8080
ENTRYPOINT ["/app/server"]
