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

// Estructura que usamos para mapear las emergencias que vienen del archivo JSON
type Emergencia struct {
    Name      string  `json:"name"`
    Latitude  float32 `json:"latitude"`
    Longitude float32 `json:"longitude"`
    Magnitude int32   `json:"magnitude"`
}

// Esta función sirve para vaciar el canal de eventos antes de empezar una nueva emergencia,
// así evitamos que se acumulen mensajes anteriores.
func drainEventos(ch <-chan *proto.ActualizacionReply) {
    for {
        select {
        case <-ch:
        default:
            return
        }
    }
}

func main() {
    // Leemos las emergencias desde un archivo JSON local
    file, err := os.ReadFile("emergencias.json")
    if err != nil {
        log.Fatalf("No se pudo leer emergencias.json: %v", err)
    }

    var emergencias []Emergencia
    if err := json.Unmarshal(file, &emergencias); err != nil {
        log.Fatalf("Error al parsear JSON: %v", err)
    }

    fmt.Println("Emergencias recibidas")

    // Establecemos conexión gRPC con el servicio de asignación
    asignacionConn, err := grpc.Dial("10.10.28.36:50051", grpc.WithInsecure())
    if err != nil {
        log.Fatalf("No se pudo conectar a Asignación: %v", err)
    }
    defer asignacionConn.Close()
    asignador := proto.NewAsignacionServiceClient(asignacionConn)

    // Conectamos también al servicio de monitoreo para recibir actualizaciones
    monitoreoConn, err := grpc.Dial("localhost:50053", grpc.WithInsecure())
    if err != nil {
        log.Fatalf("No se pudo conectar a Monitoreo: %v", err)
    }
    defer monitoreoConn.Close()
    monitoreador := proto.NewMonitoreoServiceClient(monitoreoConn)

    // Nos suscribimos a las actualizaciones del monitoreo
    stream, err := monitoreador.RecibirActualizaciones(context.Background(), &proto.ActualizacionRequest{
        ClienteId: "cliente-001",
    })
    if err != nil {
        log.Fatalf("No se pudo conectar al monitoreo: %v\n", err)
    }

    // Canal donde llegarán los eventos emitidos por RabbitMQ vía gRPC
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

    // Procesamos una a una las emergencias
    for _, e := range emergencias {
        nombre := e.Name
        x := e.Latitude
        y := e.Longitude
        mag := e.Magnitude

        fmt.Printf("\nEmergencia actual: %s magnitud %d en x = %.0f, y = %.0f\n", nombre, mag, x, y)

        // Creamos la solicitud para asignar un dron a esta emergencia
        req := &proto.EmergenciaRequest{
            Nombre:    nombre,
            Latitude:  x,
            Longitude: y,
            Magnitud:  mag,
        }

        // Definimos un contexto con timeout para evitar bloqueos
        ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
        defer cancel()

        // Canales para manejar la respuesta de asignación y posibles errores
        respChan := make(chan *proto.EmergenciaReply)
        errChan := make(chan error)

        // Pedimos que nos asignen un dron en una goroutine aparte
        go func() {
            resp, err := asignador.AsignarEmergencia(ctx, req)
            if err != nil {
                errChan <- err
                return
            }
            respChan <- resp
        }()

        done := make(chan struct{})
        ready := make(chan struct{})
        estado := ""
        var mu sync.Mutex

        // Vaciamos los mensajes anteriores, por si hay residuos
        drainEventos(eventos)

        // Esta goroutine imprime mensajes cada 3 segundos dependiendo del estado actual
        go func() {
            ticker := time.NewTicker(3 * time.Second)
            defer ticker.Stop()
            for {
                select {
                case <-done:
                    return
                case <-ticker.C:
                    mu.Lock()
                    switch estado {
                    case "En curso":
                        fmt.Println("Dron en camino a emergencia...")
                    case "Apagando":
                        fmt.Println("Dron apagando emergencia...")
                    }
                    mu.Unlock()
                }
            }
        }()

        // Aquí escuchamos los eventos de monitoreo para esta emergencia específica
        go func(nombre string) {
            close(ready) // avisamos que ya estamos listos para recibir
            for msg := range eventos {
                if strings.ToLower(msg.Ubicacion) != strings.ToLower(nombre) {
                    continue // ignoramos si el mensaje no es para esta emergencia
                }

                if msg.Estado == "Asignado" {
                    fmt.Printf("Se ha asignado %s a la emergencia\n", msg.DronId)
                    continue
                }

                mu.Lock()
                estado = msg.Estado
                mu.Unlock()

                switch msg.Estado {
                case "En curso":
                    fmt.Println("Dron en camino a emergencia...")
                case "Apagando":
                    fmt.Println("Dron apagando emergencia...")
                case "Extinguido":
                    fmt.Printf("%s ha sido extinguido por %s\n", nombre, msg.DronId)
                    close(done)
                    return
                }
            }
        }(nombre)

        <-ready // esperamos que el receptor esté listo

        // Esperamos la respuesta de la asignación
        select {
        case <-respChan:
        case err := <-errChan:
            fmt.Println("Error al asignar emergencia:", err)
            continue
        }

        <-done // esperamos que se extinga completamente
    }

    wg.Wait() 
}
