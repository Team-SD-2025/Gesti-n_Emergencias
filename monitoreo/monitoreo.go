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

func (s *monitoreoServer) RecibirActualizaciones(req *proto.ActualizacionRequest, stream proto.MonitoreoService_RecibirActualizacionesServer) error {
    log.Printf("Cliente %s conectado al monitoreo", req.ClienteId)

    updates := escucharRabbitMQ() // canal exclusivo para este cliente

    for msg := range updates {
        var data map[string]interface{}
        if err := json.Unmarshal(msg, &data); err != nil {
            continue
        }

        resp := &proto.ActualizacionReply{
            DronId:    data["dron_id"].(string),
            Estado:    data["estado"].(string),
            Ubicacion: data["ubicacion"].(string),
            Timestamp: data["timestamp"].(string),
        }

        if err := stream.Send(resp); err != nil {
            log.Printf("Error enviando al cliente: %v", err)
            break
        }
    }

    return nil
}

func escucharRabbitMQ() <-chan []byte {
    conn, err := amqp.Dial("amqp://monitoreo:monitoreo123@10.10.28.37:5672/") // IP de RabbitMQ (VM3)
    if err != nil {
        log.Fatalf("No se pudo conectar a RabbitMQ: %v", err)
    }

    ch, err := conn.Channel()
    if err != nil {
        log.Fatalf("No se pudo abrir canal: %v", err)
    }

    ch.QueueDeclare("monitoreo", true, false, false, false, nil)

    msgs, err := ch.Consume("monitoreo", "", true, false, false, false, nil)
    if err != nil {
        log.Fatalf("Error al consumir cola monitoreo: %v", err)
    }

    updates := make(chan []byte)

    go func() {
        for d := range msgs {
            updates <- d.Body
        }
    }()

    return updates
}

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

