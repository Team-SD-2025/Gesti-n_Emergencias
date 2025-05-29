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

// Vacia el canal de eventos antes de procesar una nueva emergencia
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
	// Leer archivo JSON con emergencias
	file, err := os.ReadFile("emergencias.json")
	if err != nil {
		log.Fatalf("No se pudo leer emergencias.json: %v", err)
	}

	var emergencias []Emergencia
	if err := json.Unmarshal(file, &emergencias); err != nil {
		log.Fatalf("Error al parsear JSON: %v", err)
	}

	fmt.Println("Emergencias recibidas")

	// Conexión gRPC con el servicio de asignación
	asignacionConn, err := grpc.Dial("10.10.28.36:50051", grpc.WithInsecure())
	if err != nil {
		log.Fatalf("No se pudo conectar a Asignación: %v", err)
	}
	defer asignacionConn.Close()
	asignador := proto.NewAsignacionServiceClient(asignacionConn)

	// Conexión gRPC con el servicio de monitoreo
	monitoreoConn, err := grpc.Dial("localhost:50053", grpc.WithInsecure())
	if err != nil {
		log.Fatalf("No se pudo conectar a Monitoreo: %v", err)
	}
	defer monitoreoConn.Close()
	monitoreador := proto.NewMonitoreoServiceClient(monitoreoConn)

	// Abrir canal gRPC para recibir actualizaciones de estado
	stream, err := monitoreador.RecibirActualizaciones(context.Background(), &proto.ActualizacionRequest{
		ClienteId: "cliente-001",
	})
	if err != nil {
		log.Fatalf("No se pudo conectar al monitoreo: %v\n", err)
	}

	// Canal para recibir actualizaciones del monitoreo
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

		fmt.Printf("\nEmergencia actual: %s magnitud %d en x = %.0f, y = %.0f\n", nombre, mag, x, y)

		req := &proto.EmergenciaRequest{
			Nombre:   nombre,
			Latitud:  x,
			Longitud: y,
			Magnitud: mag,
		}

		// Contexto con timeout de 3 minutos para la respuesta del servicio de asignación
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()

		// Canales para manejar respuesta o error del servicio de asignación
		respChan := make(chan *proto.EmergenciaReply)
		errChan := make(chan error)

		// Enviar la emergencia de forma concurrente
		go func() {
			resp, err := asignador.AsignarEmergencia(ctx, req)
			if err != nil {
				errChan <- err
				return
			}
			respChan <- resp
		}()

		done := make(chan struct{})  // Se cierra cuando la emergencia termina
		ready := make(chan struct{}) // Se cierra cuando comienza a monitorearse
		estado := ""
		var mu sync.Mutex

		// Limpiar eventos pendientes anteriores
		drainEventos(eventos)

		// Mostrar mensajes periódicos mientras el dron está en curso o apagando
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

		// Escucha los mensajes del monitoreo para esta emergencia específica
		go func(nombre string) {
			close(ready) // Habilita el avance del ciclo principal

			for msg := range eventos {
				if strings.ToLower(msg.Ubicacion) != strings.ToLower(nombre) {
					continue
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

		<-ready // Espera a que comience el monitoreo

		// Espera la respuesta del servicio de asignación
		select {
		case <-respChan:
		case err := <-errChan:
			fmt.Println("Error al asignar emergencia:", err)
			continue
		}

		<-done // Espera hasta que la emergencia termine (estado "Extinguido")
	}

	wg.Wait()
}
