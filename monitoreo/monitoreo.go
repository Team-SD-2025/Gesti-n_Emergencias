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

type monitoreoServer struct {
	proto.UnimplementedMonitoreoServiceServer
}

// Método gRPC que envía actualizaciones al cliente desde RabbitMQ
func (s *monitoreoServer) RecibirActualizaciones(req *proto.ActualizacionRequest, stream proto.MonitoreoService_RecibirActualizacionesServer) error {
	log.Printf("Cliente %s conectado al monitoreo", req.ClienteId)

	updates := escucharDesdeExchangeFanout(req.ClienteId)

	for msg := range updates {
		var data map[string]interface{}
		if err := json.Unmarshal(msg, &data); err != nil {
			continue // Ignora mensajes mal formateados
		}

		// Construye el mensaje de respuesta gRPC
		resp := &proto.ActualizacionReply{
			DronId:    data["dron_id"].(string),
			Estado:    data["estado"].(string),
			Ubicacion: data["ubicacion"].(string),
			Timestamp: data["timestamp"].(string),
		}

		// Envía la actualización al cliente
		if err := stream.Send(resp); err != nil {
			log.Printf("Error enviando al cliente: %v", err)
			break
		}
	}

	return nil
}

// Se conecta a RabbitMQ y escucha mensajes desde un exchange tipo fanout
func escucharDesdeExchangeFanout(clienteID string) <-chan []byte {

	// Conexión a RabbitMQ (usuario monitoreo)
	conn, err := amqp.Dial("amqp://monitoreo:monitoreo123@10.10.28.37:5672/")
	if err != nil {
		log.Fatalf("No se pudo conectar a RabbitMQ: %v", err)
	}

	ch, err := conn.Channel()
	if err != nil {
		log.Fatalf("No se pudo abrir canal: %v", err)
	}

	// 1. Declarar el exchange fanout
	err = ch.ExchangeDeclare(
		"monitoreo_exchange", "fanout", true, false, false, false, nil,
	)
	if err != nil {
		log.Fatalf("No se pudo declarar exchange: %v", err)
	}

	// 2. Crear una cola exclusiva y efímera para este cliente
	q, err := ch.QueueDeclare(
		"", false, true, true, false, nil, // nombre vacío genera una cola única
	)
	if err != nil {
		log.Fatalf("No se pudo declarar cola exclusiva: %v", err)
	}

	// 3. Vincular la cola al exchange
	err = ch.QueueBind(q.Name, "", "monitoreo_exchange", false, nil)
	if err != nil {
		log.Fatalf("No se pudo enlazar la cola al exchange: %v", err)
	}

	// 4. Comenzar a consumir mensajes de la cola
	msgs, err := ch.Consume(q.Name, "", true, true, false, false, nil)
	if err != nil {
		log.Fatalf("Error al consumir mensajes: %v", err)
	}

	updates := make(chan []byte)

	// Reenviar cada mensaje recibido al canal 'updates'
	go func() {
		for d := range msgs {
			updates <- d.Body
		}
	}()

	return updates
}

// Inicia el servidor gRPC y escucha en el puerto 50053
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
