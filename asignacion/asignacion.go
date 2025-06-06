package main

import (
    "context"
    "encoding/json"
    "fmt"
    "log"
    "math"
    "net"
    "sync"
    "time"

    "github.com/streadway/amqp"
    "go.mongodb.org/mongo-driver/bson"
    "go.mongodb.org/mongo-driver/mongo"
    "go.mongodb.org/mongo-driver/mongo/options"
    "google.golang.org/grpc"

    "Gestion_Emergencias/proto"
)

// Conexiones necesarias y constantes de configuración del sistema
const (
    mongoURI   = "mongodb://localhost:27017"
    rabbitURI  = "amqp://monitoreo:monitoreo123@10.10.28.36:5672/"
    droneGRPC  = "10.10.28.37:50052" // IP del servidor de drones
    rabbitCola = "registro"          // Cola donde se registran eventos de emergencias
)

// Servidor gRPC que maneja asignación de drones
type asignacionServer struct {
    proto.UnimplementedAsignacionServiceServer
    mongoClient  *mongo.Client     // conexión a MongoDB
    muEmergencia sync.Mutex        // mutex para evitar que dos emergencias se procesen al mismo tiempo
}

// Estructura que representa un dron guardado en la base de datos
type Dron struct {
    ID        string  `bson:"id"`
    Latitude  float32 `bson:"latitude"`
    Longitude float32 `bson:"longitude"`
    Status    string  `bson:"status"`
}

// Esta función se llama cuando llega una nueva emergencia.
// Su objetivo es buscar el dron más cercano y coordinar su activación.
func (s *asignacionServer) AsignarEmergencia(ctx context.Context, req *proto.EmergenciaRequest) (*proto.EmergenciaReply, error) {
    s.muEmergencia.Lock()
    defer s.muEmergencia.Unlock()

    log.Printf("Emergencia recibida: %s (%.2f, %.2f)", req.Nombre, req.Latitude, req.Longitude)

    // Buscamos qué dron está más cerca y disponible
    dron, err := s.buscarDronMasCercano(req.Latitude, req.Longitude)
    if err != nil {
        return nil, fmt.Errorf("error al buscar dron: %v", err)
    }

    // Marcamos el dron como ocupado en la base de datos
    _, err = s.mongoClient.Database("emergencias").Collection("drones").UpdateOne(
        context.Background(),
        bson.M{"id": dron.ID},
        bson.M{"$set": bson.M{"status": "ocupado"}},
    )
    if err != nil {
        log.Printf("No se pudo marcar el dron como ocupado: %v", err)
    }

    log.Printf("Dron asignado: %s", dron.ID)

    // Publicamos la emergencia en RabbitMQ para que se registre
    go publicarEmergenciaRegistro(req)

    // Establecemos conexión gRPC con el servidor de drones
    conn, err := grpc.Dial(droneGRPC, grpc.WithInsecure())
    if err != nil {
        return nil, fmt.Errorf("error al conectar con el dron: %v", err)
    }
    defer conn.Close()

    // Le pedimos al dron que atienda la emergencia
    droneClient := proto.NewDroneServiceClient(conn)
    ctxDrone, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
    defer cancel()

    droneResp, err := droneClient.AtenderEmergencia(ctxDrone, &proto.DroneRequest{
        DronId:         dron.ID,
        Ubicacion:      req.Nombre,
        TipoEmergencia: req.Nombre,
        Latitude:       req.Latitude,
        Longitude:      req.Longitude,
        Magnitud:       req.Magnitud,
    })
    if err != nil {
        return nil, fmt.Errorf("error en la respuesta del dron: %v", err)
    }

    log.Printf("Emergencia finalizada por dron %s. Estado: %s", dron.ID, droneResp.Estado)

    // Cuando termina, lo volvemos a marcar como disponible
    _, err = s.mongoClient.Database("emergencias").Collection("drones").UpdateOne(
        context.Background(),
        bson.M{"id": dron.ID},
        bson.M{"$set": bson.M{"status": "available"}},
    )

    return &proto.EmergenciaReply{
        Estado:       droneResp.Estado,
        DronAsignado: dron.ID,
    }, nil
}

// Esta función manda un mensaje con los detalles de la emergencia a la cola 'registro' en RabbitMQ.
// Sirve para tener un historial centralizado de emergencias.
func publicarEmergenciaRegistro(req *proto.EmergenciaRequest) {
    conn, err := amqp.Dial(rabbitURI)
    if err != nil {
        log.Printf("Error al conectar a RabbitMQ (En curso): %v", err)
        return
    }
    defer conn.Close()

    ch, err := conn.Channel()
    if err != nil {
        log.Printf("Error al abrir canal RabbitMQ: %v", err)
        return
    }
    defer ch.Close()

    ch.QueueDeclare(rabbitCola, true, false, false, false, nil)

    msg := map[string]interface{}{
        "emergency_id": time.Now().UnixNano(), // usamos un ID basado en timestamp
        "name":         req.Nombre,
        "latitude":     req.Latitude,
        "longitude":    req.Longitude,
        "magnitude":    req.Magnitud,
        "status":       "En curso",
    }

    body, _ := json.Marshal(msg)
    ch.Publish("", rabbitCola, false, false, amqp.Publishing{
        ContentType: "application/json",
        Body:        body,
    })

    log.Printf("Emergencia enviada a registro (En curso): %s", req.Nombre)
}

// Esta función busca entre todos los drones disponibles el que esté más cerca de la emergencia
func (s *asignacionServer) buscarDronMasCercano(lat, lon float32) (*Dron, error) {
    col := s.mongoClient.Database("emergencias").Collection("drones")
    cursor, err := col.Find(context.Background(), bson.M{"status": "available"})
    if err != nil {
        return nil, err
    }
    defer cursor.Close(context.Background())

    var mejor *Dron
    min := float64(1e9) // empezamos con una distancia muy grande

    for cursor.Next(context.Background()) {
        var d Dron
        if err := cursor.Decode(&d); err != nil {
            continue
        }
        dist := calcularDistancia(d.Latitude, d.Longitude, lat, lon)
        if dist < min {
            min = dist
            mejor = &d
        }
    }

    if mejor == nil {
        return nil, fmt.Errorf("no hay drones disponibles")
    }

    return mejor, nil
}

// Simple función para calcular la distancia entre dos puntos (como si fueran en un plano)
func calcularDistancia(x1, y1, x2, y2 float32) float64 {
    dx := float64(x1 - x2)
    dy := float64(y1 - y2)
    return math.Sqrt(dx*dx + dy*dy)
}

// Conecta el servidor con MongoDB
func conectarMongo() *mongo.Client {
    client, err := mongo.Connect(context.Background(), options.Client().ApplyURI(mongoURI))
    if err != nil {
        log.Fatalf("Error al conectar a MongoDB: %v", err)
    }
    return client
}

// Esta es la función principal que inicia el servidor de asignación
func main() {
    lis, err := net.Listen("tcp", ":50051")
    if err != nil {
        log.Fatalf("No se pudo escuchar en :50051: %v", err)
    }

    mongoClient := conectarMongo()

    grpcServer := grpc.NewServer()
    proto.RegisterAsignacionServiceServer(grpcServer, &asignacionServer{
        mongoClient: mongoClient,
    })

    fmt.Println("Servidor de Asignación escuchando en :50051")
    if err := grpcServer.Serve(lis); err != nil {
        log.Fatalf("Error al iniciar servidor: %v", err)
    }
}
