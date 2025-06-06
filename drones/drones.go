package main

import (
    "context"
    "encoding/json"
    "fmt"
    "log"
    "math"
    "net"
    "time"

    "Gestion_Emergencias/proto"

    "github.com/streadway/amqp"
    "go.mongodb.org/mongo-driver/bson"
    "go.mongodb.org/mongo-driver/mongo"
    "go.mongodb.org/mongo-driver/mongo/options"
    "google.golang.org/grpc"
)

// Parámetros de conexión y nombres de colas/exchanges usados
const (
    mongoURI     = "mongodb://10.10.28.36:27017"
    rabbitURI    = "amqp://monitoreo:monitoreo123@10.10.28.36:5672/"
    grpcPort     = ":50052"
    registroCola = "registro"
    monitoreoXch = "monitoreo_exchange"
)

// Estructura base para nuestro servidor de drones, donde guardamos las conexiones activas
type droneServer struct {
    proto.UnimplementedDroneServiceServer
    mongoClient *mongo.Client
    rabbitConn  *amqp.Connection
}

// Este struct representa a un dron tal como está guardado en la base de datos
type Dron struct {
    ID        string  `bson:"id"`
    Latitude  float32 `bson:"latitude"`
    Longitude float32 `bson:"longitude"`
}

// Calcula la distancia en línea recta entre dos puntos usando pitágoras
func distancia(aLat, aLon, bLat, bLon float32) float64 {
    dx := float64(aLat - bLat)
    dy := float64(aLon - bLon)
    return math.Sqrt(dx*dx + dy*dy)
}

// Esta función publica un mensaje (en formato JSON) a RabbitMQ, ya sea a un exchange o a una cola específica
func publicarEstado(ch *amqp.Channel, exchange, queue string, msg map[string]interface{}) {
    body, _ := json.Marshal(msg)

    if exchange != "" {
        ch.Publish(exchange, "", false, false, amqp.Publishing{
            ContentType: "application/json",
            Body:        body,
        })
    }

    if queue != "" {
        ch.Publish("", queue, false, false, amqp.Publishing{
            ContentType: "application/json",
            Body:        body,
        })
    }
}

// Este método es el corazón del servidor de drones: se activa cuando se asigna una emergencia
func (s *droneServer) AtenderEmergencia(ctx context.Context, req *proto.DroneRequest) (*proto.DroneReply, error) {
    // Buscamos la información del dron en MongoDB
    col := s.mongoClient.Database("emergencias").Collection("drones")
    var dron Dron
    if err := col.FindOne(ctx, bson.M{"id": req.DronId}).Decode(&dron); err != nil {
        return nil, fmt.Errorf("no se pudo encontrar dron %s en MongoDB", req.DronId)
    }

    // Calculamos cuánto tardaría el dron en llegar al lugar de la emergencia
    dist := distancia(dron.Latitude, dron.Longitude, req.Latitude, req.Longitude)
    duracionVuelo := time.Duration(dist*0.5*1000) * time.Millisecond

    log.Printf("Drone %s recibió emergencia: %s | Distancia: %.2f | Vuelo estimado: %.1f seg",
        req.DronId, req.Ubicacion, dist, duracionVuelo.Seconds())

    // Nos conectamos a RabbitMQ para reportar el estado
    ch, err := s.rabbitConn.Channel()
    if err != nil {
        return nil, fmt.Errorf("no se pudo abrir canal RabbitMQ")
    }
    defer ch.Close()

    // Aseguramos que las colas y exchange existan
    ch.QueueDeclare(registroCola, true, false, false, false, nil)
    ch.ExchangeDeclare(monitoreoXch, "fanout", true, false, false, false, nil)

    // Informamos que el dron fue asignado a una emergencia
    publicarEstado(ch, monitoreoXch, "", map[string]interface{}{
        "dron_id":   dron.ID,
        "estado":    "Asignado",
        "ubicacion": req.Ubicacion,
        "timestamp": time.Now().Format(time.RFC3339),
    })

    // Estado inicial: dron en camino
    publicarEstado(ch, monitoreoXch, "", map[string]interface{}{
        "dron_id":   dron.ID,
        "estado":    "En curso",
        "ubicacion": req.Ubicacion,
        "timestamp": time.Now().Format(time.RFC3339),
    })

    // Simulamos el vuelo del dron hasta la emergencia
    time.Sleep(duracionVuelo)

    // Llegó: comenzamos el apagado
    publicarEstado(ch, monitoreoXch, "", map[string]interface{}{
        "dron_id":   dron.ID,
        "estado":    "Apagando",
        "ubicacion": req.Ubicacion,
        "timestamp": time.Now().Format(time.RFC3339),
    })

    // Enviamos actualizaciones de estado "Apagando" cada 5 segundos
    ticker := time.NewTicker(5 * time.Second)
    done := make(chan bool)
    go func() {
        for {
            select {
            case <-done:
                return
            case <-ticker.C:
                publicarEstado(ch, monitoreoXch, "", map[string]interface{}{
                    "dron_id":   dron.ID,
                    "estado":    "Apagando",
                    "ubicacion": req.Ubicacion,
                    "timestamp": time.Now().Format(time.RFC3339),
                })
            }
        }
    }()

    // Simulamos el tiempo que se demora en extinguir el fuego (depende de la magnitud)
    time.Sleep(time.Duration(req.Magnitud*2) * time.Second)
    ticker.Stop()
    done <- true

    // Finalmente, reportamos que la emergencia ha sido apagada
    finalMsg := map[string]interface{}{
        "dron_id":   dron.ID,
        "estado":    "Extinguido",
        "ubicacion": req.Ubicacion,
        "timestamp": time.Now().Format(time.RFC3339),
    }
    publicarEstado(ch, monitoreoXch, "", finalMsg)
    publicarEstado(ch, "", registroCola, finalMsg)
    log.Println("Emergencia extinguida. Estado enviado a Registro y Monitoreo.")

    // Actualizamos la posición final del dron y lo marcamos como disponible
    updateCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
    defer cancel()
    _, err = col.UpdateOne(updateCtx, bson.M{"id": dron.ID}, bson.M{
        "$set": bson.M{
            "latitude":  req.Latitude,
            "longitude": req.Longitude,
            "status":    "available",
        },
    })
    if err != nil {
        log.Printf("No se pudo actualizar ubicación del dron en MongoDB: %v", err)
    }

    return &proto.DroneReply{Estado: "Extinguido"}, nil
}

// Abre conexión a MongoDB
func conectarMongo() *mongo.Client {
    client, err := mongo.Connect(context.Background(), options.Client().ApplyURI(mongoURI))
    if err != nil {
        log.Fatalf("Error al conectar a MongoDB: %v", err)
    }
    return client
}

// Abre conexión a RabbitMQ
func conectarRabbit() *amqp.Connection {
    conn, err := amqp.Dial(rabbitURI)
    if err != nil {
        log.Fatalf("Error al conectar a RabbitMQ: %v", err)
    }
    return conn
}

// Punto de entrada del servidor: configura conexiones y lanza el servicio gRPC
func main() {
    lis, err := net.Listen("tcp", grpcPort)
    if err != nil {
        log.Fatalf("Error al escuchar en %s: %v", grpcPort, err)
    }

    mongoClient := conectarMongo()
    rabbitConn := conectarRabbit()

    grpcServer := grpc.NewServer()
    proto.RegisterDroneServiceServer(grpcServer, &droneServer{
        mongoClient: mongoClient,
        rabbitConn:  rabbitConn,
    })

    fmt.Printf("Servidor de Drones escuchando en %s\n", grpcPort)
    if err := grpcServer.Serve(lis); err != nil {
        log.Fatalf("Error al iniciar servidor: %v", err)
    }
}
