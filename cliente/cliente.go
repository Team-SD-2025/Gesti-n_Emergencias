package main

import (
    "context"
    "encoding/json"
    "fmt"
    "log"
    "os"
    "time"

    "Gestion_Emergencias/proto"

    "google.golang.org/grpc"
)

// Estructura de emergencia
type Emergencia struct {
    Name      string  json:"name"
    Latitude  float32 json:"latitude"
    Longitude float32 json:"longitude"
    Magnitude int32   json:"magnitude"
}

func main() {
    // Leer archivo JSON
    file, err := os.ReadFile("emergencias.json")
    if err != nil {
        log.Fatalf("No se pudo leer el archivo emergencias.json: %v", err)
    }

    var emergencias []Emergencia
    if err := json.Unmarshal(file, &emergencias); err != nil {
        log.Fatalf("Error al parsear el JSON: %v", err)
    }

    // Conexión gRPC a Asignación
    conn, err := grpc.Dial("localhost:50051", grpc.WithInsecure())
    if err != nil {
        log.Fatalf("Error al conectar con Asignación: %v", err)
    }
    defer conn.Close()

    client := proto.NewAsignacionServiceClient(conn)


    for i, e := range emergencias {
        fmt.Printf("Enviando emergencia #%d: %s\n", i+1, e.Name)

        req := &proto.EmergenciaRequest{
            Nombre:    e.Name,
            Latitud:   e.Latitude,
            Longitud:  e.Longitude,
            Magnitud:  e.Magnitude,
        }

        ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
        defer cancel()

        resp, err := client.AsignarEmergencia(ctx, req)
        if err != nil {
            log.Printf("Error al enviar emergencia: %v\n", err)
            continue
        }

        fmt.Printf("Emergencia asignada a dron %s. Estado: %s\n\n", resp.DronAsignado, resp.Estado)
    }

    fmt.Println("Todas las emergencias fueron enviadas.")
}