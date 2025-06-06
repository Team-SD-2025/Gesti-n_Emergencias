package main

import (
        "encoding/json"
        "fmt"
        "log"
        "net"

        "Gestion_Emergencias/proto"

        "github.com/streadway/amqp"
        "google.golang.org/grpc"
)

const (
        rabbitURI        = "amqp://monitoreo:monitoreo123@10.10.28.36:5672/"
        monitoreoExchange = "monitoreo_exchange"
)

// Servidor de monitoreo implementado en Go, que consume mensajes desde RabbitMQ
// y los reenvía en tiempo real al cliente vía gRPC.
type monitoreoServer struct {
        proto.UnimplementedMonitoreoServiceServer
}

// Maneja el stream de actualizaciones desde RabbitMQ hacia el cliente conectado por gRPC.
func (s *monitoreoServer) RecibirActualizaciones(req *proto.ActualizacionRequest, stream proto.MonitoreoService_RecibirActualizacionesServer) error {
        log.Printf("Cliente %s conectado al monitoreo", req.ClienteId)

        updates := escucharDesdeExchangeFanout(req.ClienteId)

        for msg := range updates {
                var data map[string]interface{}
                if err := json.Unmarshal(msg, &data); err != nil {
                        continue
                }

                // Extracción segura de campos
                dronID, _ := data["dron_id"].(string)
                estado, _ := data["estado"].(string)
                ubicacion, _ := data["ubicacion"].(string)
                timestamp, _ := data["timestamp"].(string)

                resp := &proto.ActualizacionReply{
                        DronId:    dronID,
                        Estado:    estado,
                        Ubicacion: ubicacion,
                        Timestamp: timestamp,
                }

                if err := stream.Send(resp); err != nil {
                        log.Printf("Error enviando actualización al cliente: %v", err)
                        break
                }
        }

        return nil
}

// Escucha desde el exchange fanout y reenvía los mensajes recibidos en un canal.
func escucharDesdeExchangeFanout(clienteID string) <-chan []byte {
        conn, err := amqp.Dial(rabbitURI)
        if err != nil {
                log.Fatalf("No se pudo conectar a RabbitMQ: %v", err)
        }

        ch, err := conn.Channel()
        if err != nil {
                log.Fatalf("No se pudo abrir canal: %v", err)
        }

        err = ch.ExchangeDeclare(
                monitoreoExchange, "fanout", true, false, false, false, nil,
        )
        if err != nil {
                log.Fatalf("No se pudo declarar exchange: %v", err)
        }

        q, err := ch.QueueDeclare(
                "", false, true, true, false, nil,
        )
        if err != nil {
                log.Fatalf("No se pudo declarar cola exclusiva: %v", err)
        }

        err = ch.QueueBind(q.Name, "", monitoreoExchange, false, nil)
        if err != nil {
                log.Fatalf("No se pudo enlazar la cola al exchange: %v", err)
        }

        msgs, err := ch.Consume(q.Name, "", true, true, false, false, nil)
        if err != nil {
                log.Fatalf("Error al consumir mensajes: %v", err)
        }

        updates := make(chan []byte)

        // Reenviar mensajes desde RabbitMQ al canal
        go func() {
                defer conn.Close()
                defer ch.Close()

                for d := range msgs {
                        updates <- d.Body
                }
                close(updates)
        }()

        return updates
}

// Inicializa el servidor gRPC en el puerto 50053.
func main() {
        lis, err := net.Listen("tcp", ":50053")
        if err != nil {
                log.Fatalf("No se pudo escuchar en el puerto 50053: %v", err)
        }

        grpcServer := grpc.NewServer()
        proto.RegisterMonitoreoServiceServer(grpcServer, &monitoreoServer{})

        fmt.Println("Servicio de Monitoreo escuchando en :50053")
        if err := grpcServer.Serve(lis); err != nil {
                log.Fatalf("Error al iniciar el servidor gRPC: %v", err)
        }
}