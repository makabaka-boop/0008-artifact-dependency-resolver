# ---- build stage ----
FROM golang:1.22-alpine AS build
RUN apk add --no-cache ca-certificates git
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/server ./cmd/server

# ---- runtime stage ----
FROM alpine:3.20
RUN apk add --no-cache ca-certificates && adduser -D -u 10001 app
USER app
WORKDIR /app
COPY --from=build /out/server /app/server
RUN mkdir -p /app/data && chown -R app:app /app/data
EXPOSE 8080
ENV LISTEN_ADDR=:8080
ENV DB_PATH=/app/data/app.db
ENTRYPOINT ["/app/server"]
