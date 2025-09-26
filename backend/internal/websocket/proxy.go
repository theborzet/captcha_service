package websocket

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/gorilla/websocket"
	pb "github.com/theborzet/captcha_service/pkg/api/pb/captcha/v1"
)

// Proxy - проксирует WebSocket-соединение между браузером и gRPC-сервером капчи.
type Proxy struct {
	client pb.CaptchaServiceClient
	log    *slog.Logger
	ctx    context.Context // Добавлен контекст
}

// NewProxy создаёт новый WebSocket-прокси.
func NewProxy(client pb.CaptchaServiceClient, log *slog.Logger, ctx context.Context) *Proxy {
	return &Proxy{
		client: client,
		log:    log,
		ctx:    ctx,
	}
}

// ServeHTTP обрабатывает входящие WebSocket-запросы и проксирует события
// между клиентом (браузером) и gRPC-сервисом капчи.
func (p *Proxy) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	upgrader := websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool {
			return true
		},
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		p.log.Error("Failed to upgrade to WebSocket", slog.Any("error", err))
		return
	}
	defer conn.Close()

	p.log.Info("WebSocket connection established")

	// Используем переданный контекст с возможностью отмены
	ctx, cancel := context.WithCancel(p.ctx)
	defer cancel()

	// Открываем стрим к gRPC серверу
	stream, err := p.client.MakeEventStream(ctx)
	if err != nil {
		p.log.Error("Failed to open gRPC stream", slog.Any("error", err))
		return
	}
	defer stream.CloseSend()

	// Читаем сообщения от клиента
	go func() {
		defer cancel() // Отменяем контекст при завершении
		for {
			_, message, err := conn.ReadMessage()
			if err != nil {
				p.log.Info("Client disconnected", slog.Any("error", err))
				return
			}

			p.log.Info("Received raw message from client", slog.String("raw_message", string(message))) // Логируем сырое сообщение

			// Парсим событие
			var event struct {
				Type string `json:"type"`
				Data string `json:"data"`
			}
			if err := json.Unmarshal(message, &event); err != nil {
				p.log.Warn("Failed to parse event", slog.Any("error", err), slog.String("raw_message", string(message)))
				continue
			}

			// Проверяем, что данные не пустые
			if event.Type == "" || event.Data == "" {
				p.log.Warn("Invalid event data", slog.String("type", event.Type), slog.String("data", event.Data))
				continue
			}

			p.log.Info("Processed event", slog.String("type", event.Type), slog.String("data", event.Data)) // Логируем обработанное событие

			// Отправляем на gRPC сервер
			clientEvent := &pb.ClientEvent{
				EventType: pb.ClientEvent_FRONTEND_EVENT,
				Data:      []byte(event.Data),
			}

			if err := stream.Send(clientEvent); err != nil {
				p.log.Error("Failed to send event to gRPC server", slog.Any("error", err))
				return
			}
		}
	}()

	// Читаем ответы от gRPC сервера
	for {
		select {
		case <-ctx.Done():
			p.log.Info("gRPC stream closed by context")
			return
		default:
			serverEvent, err := stream.Recv()
			if err != nil {
				p.log.Info("gRPC server disconnected", slog.Any("error", err))
				return
			}

			// Отправляем ответ клиенту
			response, err := json.Marshal(serverEvent)
			if err != nil {
				p.log.Warn("Failed to serialize response", slog.Any("error", err))
				continue
			}

			if err := conn.WriteMessage(websocket.TextMessage, response); err != nil {
				p.log.Info("Client disconnected", slog.Any("error", err))
				return
			}
		}
	}
}
