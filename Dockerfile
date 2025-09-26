FROM golang:1.24-alpine AS builder

WORKDIR /app

COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Собираем бинарник
RUN CGO_ENABLED=0 GOOS=linux go build -o /captcha-service ./backend/cmd/v1/main.go

FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /root/

COPY --from=builder /captcha-service .
COPY backend/config/ ./config/
COPY .env .

EXPOSE 38000-40000

CMD ["./captcha-service", "-config", "./config/app.yaml"]