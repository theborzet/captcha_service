.PHONY: proto generate run build clean docker-build docker-run docker-down

# Генерация gRPC кода
proto:
	@./scripts/generate-proto.sh

generate: proto

# Запуск локально
back-run:
	@cd backend/cmd/v1 && go run main.go -config ../../config/app.yaml

front-run:
	@cd frontend/public/drag-drop && python3 -m http.server 8000

balancer-run:
	@cd backend/balancer-mock && go run main.go

build:
	@cd backend/cmd/v1 && go build -o ../../captcha-service main.go

clean:
	@rm -rf backend/pkg/api/pb/*
	@rm -f backend/cmd/v1/captcha-service

docker-build:
	@docker build -t captcha-service .

docker-run:
	@docker-compose up --build

docker-down:
	@docker-compose down