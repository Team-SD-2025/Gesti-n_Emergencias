package main

import (
	"context"
	"fmt"
	"log"
	"math"
	"net"
	"sync"
	"time"

	"Gestion_Emergencias/proto"
	"encoding/json"

	"github.com/streadway/amqp"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"google.golang.org/grpc"
)

// Estructura del servidor de asignación
type asignacionServer struct {
	proto.UnimplementedAsignacionServiceServer
	mongoClient  *mongo.Client
	muEmergencia sync.Mutex // Mutex para evitar asignaciones simultáneas
}

// Estructura que representa un dron en MongoDB
type Dron struct {
	ID       string  `bson:"id"`
	Latitud  float32 `bson:"latitud"`
	Longitud float32 `bson:"longitud"`
}

// Método gRPC que maneja la asignación de emergencias
func (s *asignacionServer) AsignarEmergencia(ctx context.Context, req *proto.EmergenciaRequest) (*proto.EmergenciaReply, error) {
	// Evita que dos emergencias se asignen al mismo tiempo
	s.muEmergencia.Lock()
	defer s.muEmergencia.Unlock()

	log.Printf("Emergencia recibida: %s (%.4f, %.4f)", req.Nombre, req.Latitud, req.Longitud)

	// Buscar el dron más cercano a la ubicación de la emergencia
	dron, err := s.buscarDronMasCercano(req.Latitud, req.Longitud)
	if err != nil {
		return nil, fmt.Errorf("error al buscar dron: %v", err)
	}

	// Marcar el dron como "ocupado" en MongoDB
	_, err = s.mongoClient.Database("emergencias").Collection("drones").UpdateOne(
		context.TODO(),
		bson.M{"id": dron.ID},
		bson.M{"$set": bson.M{"status": "ocupado"}},
	)
	if err != nil {
		log.Printf("No se pudo marcar el dron como ocupado: %v", err)
	}

	log.Printf("Dron asignado: %s", dron.ID)

	// Enviar mensaje de estado "Atendida" al servicio de registro vía RabbitMQ
	go func() {
		conn, err := amqp.Dial("amqp://monitoreo:monitoreo123@10.10.28.37:5672/")
		if err != nil {
			log.Printf("Error al conectar a RabbitMQ: %v", err)
			return
		}
		defer conn.Close()

		ch, err := conn.Channel()
		if err != nil {
			log.Printf("Error al abrir canal RabbitMQ: %v", err)
			return
		}
		defer ch.Close()

		// Asegura que la cola exista
		ch.QueueDeclare("registro", true, false, false, false, nil)

		// Crear mensaje JSON con estado "Atendida"
		msg := map[string]interface{}{
			"dron_id":   dron.ID,
			"estado":    "Atendida",
			"ubicacion": req.Nombre,
			"timestamp": time.Now().Format(time.RFC3339),
		}

		body, _ := json.Marshal(msg)
		ch.Publish("", "registro", false, false, amqp.Publishing{
			ContentType: "application/json",
			Body:        body,
		})

		log.Printf("Estado 'Atendida' enviado a registro.")
	}()

	// Llama al servicio del dron vía gRPC para que atienda la emergencia
	conn, err := grpc.Dial("10.10.28.37:50052", grpc.WithInsecure())
	if err != nil {
		return nil, fmt.Errorf("error al conectar con el dron: %v", err)
	}
	defer conn.Close()

	droneClient := proto.NewDroneServiceClient(conn)

	// Construir el mensaje gRPC con los datos de la emergencia
	dronReq := &proto.DroneRequest{
		DronId:         dron.ID,
		Ubicacion:      req.Nombre,
		TipoEmergencia: req.Nombre,
		Latitud:        req.Latitud,
		Longitud:       req.Longitud,
		Magnitud:       req.Magnitud,
	}

	// Establece timeout de 3 minutos para la acción del dron
	ctxDrone, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()

	// Envía la emergencia al dron
	droneResp, err := droneClient.AtenderEmergencia(ctxDrone, dronReq)
	if err != nil {
		return nil, fmt.Errorf("error en la respuesta del dron: %v", err)
	}

	log.Printf("Emergencia atendida por dron %s. Estado: %s", dron.ID, droneResp.Estado)

	// Devuelve respuesta al cliente con el estado y el dron asignado
	return &proto.EmergenciaReply{
		Estado:       droneResp.Estado,
		DronAsignado: dron.ID,
	}, nil
}

// busca el dron disponible más cercano a una ubicación (lat, lon)
func (s *asignacionServer) buscarDronMasCercano(lat float32, lon float32) (*Dron, error) {

	// Busca drones disponibles
	col := s.mongoClient.Database("emergencias").Collection("drones")

	cursor, err := col.Find(context.TODO(), bson.M{"status": "available"})
	if err != nil {
		return nil, err
	}
	defer cursor.Close(context.TODO())

	var dronMasCercano *Dron
	minDist := float64(9999999)

	// Recorre los drones y calcula distancia
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

// Calcula distancia euclídea entre dos puntos geográficos (plano)
func calcularDistancia(lat1, lon1, lat2, lon2 float32) float64 {
	dx := float64(lat2 - lat1)
	dy := float64(lon2 - lon1)
	return math.Sqrt(dx*dx + dy*dy)
}

// Conexión básica a MongoDB local
func conectarMongo() *mongo.Client {
	client, err := mongo.Connect(context.TODO(), options.Client().ApplyURI("mongodb://localhost:27017"))
	if err != nil {
		log.Fatalf("Error al conectar con MongoDB: %v", err)
	}
	return client
}

// Función main: levanta servidor gRPC en el puerto 50051
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
