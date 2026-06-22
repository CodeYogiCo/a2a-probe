# ── build stage ───────────────────────────────────────────────────────────────
FROM golang:1.23-alpine AS builder
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o a2a-probe .

# ── runtime stage ─────────────────────────────────────────────────────────────
FROM scratch
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /build/a2a-probe /a2a-probe
ENTRYPOINT ["/a2a-probe"]
