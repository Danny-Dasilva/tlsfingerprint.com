FROM golang:1.24-alpine AS builder

RUN apk add --no-cache build-base libpcap-dev
WORKDIR /src

COPY go.mod go.sum ./

COPY cmd ./cmd
COPY pkg ./pkg
COPY static ./static
COPY blockedIPs ./blockedIPs
COPY config.example.json ./config.example.json

# Create placeholder certs dir for build (real certs mounted at runtime)
RUN mkdir -p certs

# Tidy and download dependencies (handles new packages like gorilla/websocket)
RUN go mod tidy && go mod download

RUN CGO_ENABLED=1 go build -o /out/tlsfingerprint ./cmd/main.go

FROM alpine:3.20

RUN apk add --no-cache ca-certificates libpcap
WORKDIR /app

COPY --from=builder /out/tlsfingerprint ./tlsfingerprint
COPY --from=builder /src/static ./static
COPY --from=builder /src/blockedIPs ./blockedIPs
COPY --from=builder /src/config.example.json ./config.example.json

# Create placeholder certs dir (real certs mounted at runtime)
RUN mkdir -p certs
RUN if [ ! -f /app/config.json ]; then cp /app/config.example.json /app/config.json; fi

EXPOSE 80 443 443/udp
CMD ["./tlsfingerprint"]
