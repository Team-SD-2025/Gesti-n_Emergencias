package main

import (
    "context"
    "encoding/json"
    "fmt"
    "log"
    "os"
    "strings"
    "sync"
    "time"

    "Gestion_Emergencias/proto"

    "google.golang.org/grpc"
)

type Emergencia struct {
    Name      string  `json:"name"`
    Latitude  float32 `json:"latitude"`
    Longitude float32 `json:"longitude"`
    Magnitude int32   `json:"magnitude"`
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

    fmt.Println("Emergencias recibidas")

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

    stream, err := monitoreador.RecibirActualizaciones(context.Background(), &proto.ActualizacionRequest{
        ClienteId: "cliente-001",
    })
    if err != nil {
        log.Fatalf("No se pudo conectar al monitoreo: %v\n", err)
    }

    eventos := make(chan *proto.ActualizacionReply)

    go func() {
        for {
            msg, err := stream.Recv()
            if err != nil {
                close(eventos)
                return
            }
            eventos <- msg
        }
    }()

    var wg sync.WaitGroup

    for _, e := range emergencias {
        nombre := e.Name
        x := e.Latitude
        y := e.Longitude
        mag := e.Magnitude

        fmt.Printf("Emergencia actual: %s magnitud %d en x = %.0f, y = %.0f\n", nombre, mag, x, y)

        req := &proto.EmergenciaRequest{
            Nombre:   nombre,
            Latitud:  x,
            Longitud: y,
            Magnitud: mag,
        }

        ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
        defer cancel()

        resp, err := asignador.AsignarEmergencia(ctx, req)
        if err != nil {
            fmt.Println("Error al asignar emergencia:", err)
            continue
        }

        fmt.Printf("Se ha asignado %s a la emergencia\n", resp.DronAsignado)

        done := make(chan struct{})
        var mu sync.Mutex
        estado := "En curso"

        //imprimir estado cada 3 segundos desde la asignación
        wg.Add(1)
        go func() {
            defer wg.Done()
            for {
                select {
                case <-done:
                    return
                default:
                    mu.Lock()
                    est := estado
                    mu.Unlock()

                    switch est {
                    case "En curso":
                        fmt.Println("Dron en camino a emergencia...")
                    case "Apagando":
                        fmt.Println("Dron apagando emergencia...")
                    }
                    time.Sleep(3 * time.Second)
                }
            }
        }()

        // recibir actualizaciones y actualizar estado
        go func(nombre string) {
            for msg := range eventos {
                if strings.ToLower(msg.Ubicacion) != strings.ToLower(nombre) {
                    continue
                }

                mu.Lock()
                estado = msg.Estado
                mu.Unlock()

                if msg.Estado == "Extinguido" {
                    fmt.Printf("%s ha sido extinguido por %s\n", nombre, msg.DronId)
                    close(done)
                    return
                }
            }
        }(nombre)

        <-done
    }

    wg.Wait()
}
