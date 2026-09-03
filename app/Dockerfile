# --- Étape 1 : build ---
FROM golang:1.25-alpine AS builder

WORKDIR /src

COPY go.mod ./
RUN go mod download 2>/dev/null || true

COPY . .

ARG VERSION=dev
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w -X main.Version=${VERSION}" \
    -o /app main.go

# --- Étape 2 : image finale minimaliste ---
FROM gcr.io/distroless/static-debian12

COPY --from=builder /app /app

USER nonroot:nonroot
EXPOSE 8080

ENTRYPOINT ["/app"]