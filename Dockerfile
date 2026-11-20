FROM golang:1.22-alpine AS builder
WORKDIR /src
COPY go.mod go.sum* ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -ldflags="-s -w" -o /out/organism ./cmd/organism/...

FROM alpine:3.20
RUN apk --no-cache add ca-certificates wget \
    && mkdir -p /var/lib/organism/stratos
COPY --from=builder /out/organism /usr/local/bin/organism
EXPOSE 8080
VOLUME ["/var/lib/organism/stratos"]
ENTRYPOINT ["/usr/local/bin/organism"]
