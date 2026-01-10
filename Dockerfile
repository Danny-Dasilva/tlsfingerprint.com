FROM golang:1.23-alpine AS builder

RUN apk add --no-cache build-base libpcap-dev
WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY cmd ./cmd
COPY pkg ./pkg
COPY static ./static
COPY certs ./certs
COPY blockedIPs ./blockedIPs
COPY config.example.json ./config.example.json

RUN CGO_ENABLED=1 go build -o /out/tlsfingerprint ./cmd/main.go

FROM alpine:3.20

RUN apk add --no-cache ca-certificates libpcap
WORKDIR /app

COPY --from=builder /out/tlsfingerprint ./tlsfingerprint
COPY --from=builder /src/static ./static
COPY --from=builder /src/certs ./certs
COPY --from=builder /src/blockedIPs ./blockedIPs
COPY --from=builder /src/config.example.json ./config.example.json
RUN if [ ! -f /app/config.json ]; then cp /app/config.example.json /app/config.json; fi

EXPOSE 80 443 443/udp
CMD ["./tlsfingerprint"]
