FROM golang:1.24-alpine AS builder
WORKDIR /app
COPY main.go index.html ./
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o explorer main.go

FROM alpine:3.20
COPY --from=builder /app/explorer /usr/local/bin/explorer
EXPOSE 8080
CMD ["explorer"]
