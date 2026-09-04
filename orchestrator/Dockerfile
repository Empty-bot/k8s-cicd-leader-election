# --- Étape 1 : build ---
FROM golang:1.25-alpine AS builder

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG VERSION=dev
RUN CGO_ENABLED=0 GOOS=linux go build \
    -ldflags="-s -w -X main.Version=${VERSION}" \
    -o /orchestrator .

# --- Étape 2 : image finale minimaliste ---
FROM gcr.io/distroless/static-debian12

COPY --from=builder /orchestrator /orchestrator

USER nonroot:nonroot

ENTRYPOINT ["/orchestrator"]