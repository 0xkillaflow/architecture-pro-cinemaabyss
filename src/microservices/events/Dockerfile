FROM golang:1.27-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -o events-service .

FROM alpine:3.24

RUN apk --no-cache add ca-certificates

WORKDIR /app
COPY --from=builder /app/events-service .

EXPOSE 8082

CMD ["./events-service"]
