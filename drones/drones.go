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

type droneServer struct {
    proto.UnimplementedDroneServiceServer
    mongoClient *mongo.Client
    rabbitConn  *amqp.Connection
}

type Dron struct {
    ID       string  `bson:"id"`
    Latitud  float32 `bson:"latitud"`
    Longitud float32 `bson:"longitud"`
}

//Utilidad: distancia euclidiana
func distancia(aLat, aLon, bLat, bLon float32) float64 {
    dx := float64(aLat - bLat)
    dy := float64(aLon - bLon)
    return math.Sqrt(dx*dx + dy*dy)
}

func (s *droneServer) AtenderEmergencia(ctx context.Context, req *proto.DroneRequest) (*proto.DroneReply, error) {
    log.Printf("Drone %s recibió emergencia: %s", req.DronId, req.Ubicacion)

    //Obteniene la ubicación actual del dron
    col := s.mongoClient.Database("emergencias").Collection("drones")
    var dron Dron
    if err := col.FindOne(ctx, bson.M{"id": req.DronId}).Decode(&dron); err != nil {
        return nil, fmt.Errorf("no se pudo encontrar dron %s en MongoDB", req.DronId)
    }

    //Calcula la distancia y tiempo de vuelo
    dist := distancia(dron.Latitud, dron.Longitud, req.Latitud, req.Longitud)
    vuelo := time.Duration(dist*0.5*1000) * time.Millisecond
    log.Printf("Dron %s volando %.2f unidades (~%.1fs)", dron.ID, dist, vuelo.Seconds())

    time.Sleep(vuelo)

	ch.QueueDeclare("registro", true, false, false, false, nil)
	ch.QueueDeclare("monitoreo", true, false, false, false, nil)


    ch, err := s.rabbitConn.Channel()
    if err != nil {
        return nil, fmt.Errorf("no se pudo abrir canal en RabbitMQ")
    }
    defer ch.Close()

    ticker := time.NewTicker(5 * time.Second)
    done := make(chan bool)

    go func() {
        for {
            select {
            case <-done:
                return
            case <-ticker.C:
                msg := map[string]interface{}{
                    "dron_id":  dron.ID,
                    "estado":   "En curso",
                    "ubicacion": req.Ubicacion,
                    "timestamp": time.Now().Format(time.RFC3339),
                }
                body, _ := json.Marshal(msg)
                ch.Publish("", "monitoreo", false, false, amqp.Publishing{
                    ContentType: "application/json",
                    Body:        body,
                })
                log.Printf("📡 Enviada actualización de estado a monitoreo.")
            }
        }
    }()

    apagado := time.Duration(req.Magnitud*2) * time.Second
    log.Printf("Apagando fuego de magnitud %d (~%ds)...", req.Magnitud, apagado/time.Second)
    time.Sleep(apagado)


    ticker.Stop()
    done <- true

    //Envia mensaje final a Registro
    final := map[string]interface{}{
        "dron_id":   dron.ID,
        "estado":    "Extinguido",
        "ubicacion": req.Ubicacion,
        "timestamp": time.Now().Format(time.RFC3339),
    }
    finalBody, _ := json.Marshal(final)
    ch.Publish("", "registro", false, false, amqp.Publishing{
        ContentType: "application/json",
        Body:        finalBody,
    })
    log.Println("Emergencia extinguida. Estado enviado a Registro.")

    //Actualiza posición del dron 
    _, err = col.UpdateOne(ctx, bson.M{"id": dron.ID}, bson.M{
        "$set": bson.M{"latitud": req.Latitud, "longitud": req.Longitud},
    })
    if err != nil {
        log.Printf("No se pudo actualizar ubicación del dron en MongoDB: %v", err)
    }

    return &proto.DroneReply{Estado: "Extinguido"}, nil
}

func conectarMongo() *mongo.Client {
    client, err := mongo.Connect(context.TODO(), options.Client().ApplyURI("mongodb://10.10.28.36:27017"))

    if err != nil {
        log.Fatalf("Error al conectar a MongoDB: %v", err)
    }
    return client
}

func conectarRabbit() *amqp.Connection {
    conn, err := amqp.Dial("amqp://guest:guest@localhost:5672/")
    if err != nil {
        log.Fatalf("Error al conectar a RabbitMQ: %v", err)
    }
    return conn
}

func main() {
    lis, err := net.Listen("tcp", ":50052")
    if err != nil {
        log.Fatalf("Error al escuchar en :50052: %v", err)
    }

    mongoClient := conectarMongo()
    rabbitConn := conectarRabbit()

    grpcServer := grpc.NewServer()
    proto.RegisterDroneServiceServer(grpcServer, &droneServer{
        mongoClient: mongoClient,
        rabbitConn:  rabbitConn,
    })

    fmt.Println("Servidor de Drones escuchando en :50052")
    if err := grpcServer.Serve(lis); err != nil {
        log.Fatalf("Error al iniciar servidor: %v", err)
    }
}

