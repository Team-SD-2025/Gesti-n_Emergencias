package main

import (
    "context"
    "encoding/json"
    "fmt"
    "io"
    "log"
    "os"
    "time"

    "Gestion_Emergencias/proto"

    "google.golang.org/grpc"
)


type Emergencia struct {
    Name      string  json:"name"
    Latitude  float32 json:"latitude"
    Longitude float32 json:"longitude"
    Magnitude int32   json:"magnitude"
}

func main() {
    file, err := os.ReadFile("emergencias.json")
    if err != nil {
        log.Fatalf("No se pudo leer emergencias.json: %v", err)
    }

    var emergencias []Emergencia
    if err := json.Unmarshal(file, &emergencias); err != nil {
        log.Fatalf("Error al parsear JSON: %v", err)
    }

    asignacionConn, err := grpc.Dial("10.10.28.36:50051", grpc.WithInsecure())
    if err != nil {
        log.Fatalf("No se pudo conectar a Asignación: %v", err)
    }
    defer asignacionConn.Close()
    asignador := proto.NewAsignacionServiceClient(asignacionConn)

    monitoreoConn, err := grpc.Dial("localhost:50053", grpc.WithInsecure())
    if err != nil {
        log.Fatalf("No se pudo conectar a Monitoreo: %v", err)
    }
    defer monitoreoConn.Close()
    monitoreador := proto.NewMonitoreoServiceClient(monitoreoConn)

    go escucharMonitoreo(monitoreador)

    for i, e := range emergencias {
        fmt.Printf("Enviando emergencia #%d: %s\n", i+1, e.Name)

        req := &proto.EmergenciaRequest{
            Nombre:    e.Name,
            Latitud:   e.Latitude,
            Longitud:  e.Longitude,
            Magnitud:  e.Magnitude,
        }

        ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
        defer cancel()

        resp, err := asignador.AsignarEmergencia(ctx, req)
        if err != nil {
            log.Printf("Error al enviar emergencia: %v\n", err)
            continue
        }

        fmt.Printf("Asignada a dron %s (estado: %s)\n\n", resp.DronAsignado, resp.Estado)
    }


    fmt.Println("Esperando actualizaciones desde Monitoreo...")
    time.Sleep(20 * time.Second)
}

func escucharMonitoreo(cliente proto.MonitoreoServiceClient) {
    req := &proto.ActualizacionRequest{
        ClienteId: "cliente-001",
    }

    stream, err := cliente.RecibirActualizaciones(context.Background(), req)
    if err != nil {
        log.Printf("Error al conectar con Monitoreo: %v", err)
        return
    }

    for {
        msg, err := stream.Recv()
        if err == io.EOF {
            break
        }
        if err != nil {
            log.Printf("Error en stream: %v", err)
            break
        }

        fmt.Printf("[Monitoreo] Dron %s – %s en %s (%s)\n", msg.DronId, msg.Estado, msg.Ubicacion, msg.Timestamp)
    }
}