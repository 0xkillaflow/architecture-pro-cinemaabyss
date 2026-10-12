FROM golang:1.27-alpine AS builder

WORKDIR /app

COPY go.mod ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -o proxy-service .

FROM alpine:3.24

RUN apk --no-cache add ca-certificates

WORKDIR /app
COPY --from=builder /app/proxy-service .

EXPOSE 8000

CMD ["./proxy-service"]
