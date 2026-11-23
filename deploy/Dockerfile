FROM golang:1.25-alpine AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/raglex-server ./cmd/raglex-server

FROM alpine:3.21
RUN apk add --no-cache ca-certificates tzdata
WORKDIR /app
COPY --from=builder /out/raglex-server /usr/local/bin/raglex-server
VOLUME ["/data/library"]
EXPOSE 41595
ENTRYPOINT ["raglex-server"]
CMD ["-addr", ":41595", "-library", "/data/library"]
