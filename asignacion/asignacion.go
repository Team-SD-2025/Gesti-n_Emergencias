package main


import (
	"sync"
    "context"
    "fmt"
    "log"
    "math"
    "net"
    "time"

    "Gestion_Emergencias/proto"

    "google.golang.org/grpc"
    "go.mongodb.org/mongo-driver/bson"
    "go.mongodb.org/mongo-driver/mongo"
    "go.mongodb.org/mongo-driver/mongo/options"
)



type asignacionServer struct {
    proto.UnimplementedAsignacionServiceServer
    mongoClient *mongo.Client
    mu          sync.Mutex
}

type Dron struct {
    ID       string  `bson:"id"`
    Latitud  float32 `bson:"latitud"`
    Longitud float32 `bson:"longitud"`
}

func (s *asignacionServer) AsignarEmergencia(ctx context.Context, req *proto.EmergenciaRequest) (*proto.EmergenciaReply, error) {

	s.mu.Lock()
    defer s.mu.Unlock()

    log.Printf("Emergencia recibida: %s (%.4f, %.4f)", req.Nombre, req.Latitud, req.Longitud)

    // Buscar dron más cercano desde MongoDB
    dron, err := s.buscarDronMasCercano(req.Latitud, req.Longitud)
    if err != nil {
        return nil, fmt.Errorf("error al buscar dron: %v", err)
    }

    log.Printf("🚁 Dron asignado: %s", dron.ID)

    // Conexión al dron (localhost:50052)
    conn, err := grpc.Dial("localhost:50052", grpc.WithInsecure())
    if err != nil {
        return nil, fmt.Errorf("error al conectar con el dron: %v", err)
    }
    defer conn.Close()

    droneClient := proto.NewDroneServiceClient(conn)

    // Enviar emergencia
    dronReq := &proto.DroneRequest{
        DronId:         dron.ID,
        Ubicacion:      req.Nombre,
        TipoEmergencia: req.Nombre,
        Latitud:        req.Latitud,
        Longitud:       req.Longitud,
        Magnitud:       req.Magnitud,
    }

    // Esperar respuesta del dron
    droneResp, err := droneClient.AtenderEmergencia(ctx, dronReq)
    if err != nil {
        return nil, fmt.Errorf("error en la respuesta del dron: %v", err)
    }

    log.Printf("Emergencia atendida por dron %s. Estado: %s", dron.ID, droneResp.Estado)

    return &proto.EmergenciaReply{
        Estado:       droneResp.Estado,
        DronAsignado: dron.ID,
    }, nil
}

func (s *asignacionServer) buscarDronMasCercano(lat float32, lon float32) (*Dron, error) {
    col := s.mongoClient.Database("emergencias").Collection("drones")

    cursor, err := col.Find(context.TODO(), bson.M{"status": "available"})
    if err != nil {
        return nil, err
    }
    defer cursor.Close(context.TODO())

    var dronMasCercano *Dron
    minDist := float64(9999999)

    for cursor.Next(context.TODO()) {
        var d Dron
        if err := cursor.Decode(&d); err != nil {
            continue
        }
        dist := calcularDistancia(lat, lon, d.Latitud, d.Longitud)
        if dist < minDist {
            minDist = dist
            dronMasCercano = &d
        }
    }

    if dronMasCercano == nil {
        return nil, fmt.Errorf("no hay drones disponibles")
    }
    return dronMasCercano, nil
}

func calcularDistancia(lat1, lon1, lat2, lon2 float32) float64 {
    dx := float64(lat2 - lat1)
    dy := float64(lon2 - lon1)
    return math.Sqrt(dx*dx + dy*dy)
}

func conectarMongo() *mongo.Client {
    client, err := mongo.Connect(context.TODO(), options.Client().ApplyURI("mongodb://localhost:27017"))
    if err != nil {
        log.Fatalf("Error al conectar con MongoDB: %v", err)
    }
    return client
}

func main() {
    lis, err := net.Listen("tcp", ":50051")
    if err != nil {
        log.Fatalf("No se pudo escuchar en :50051: %v", err)
    }

    grpcServer := grpc.NewServer()
    mongoClient := conectarMongo()

    proto.RegisterAsignacionServiceServer(grpcServer, &asignacionServer{mongoClient: mongoClient})

    fmt.Println("Servidor de Asignación escuchando en :50051")
    if err := grpcServer.Serve(lis); err != nil {
        log.Fatalf("Error al iniciar servidor: %v", err)
    }
}
