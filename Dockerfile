# syntax=docker/dockerfile:1

# ---- build stage ----
FROM golang:1.25-alpine AS build
WORKDIR /src

# Cache deps first.
COPY go.mod go.sum* ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o /out/gateway ./cmd/gateway && \
    CGO_ENABLED=0 GOOS=linux go build -o /out/migrate ./cmd/migrate

# ---- runtime stage ----
FROM alpine:3.20
RUN apk add --no-cache ca-certificates && adduser -D -u 10001 app
WORKDIR /app

COPY --from=build /out/gateway /app/gateway
COPY --from=build /out/migrate /app/migrate
COPY migrations /app/migrations
COPY pricing.yaml /app/pricing.yaml

USER app
EXPOSE 8080
ENTRYPOINT ["/app/gateway"]
