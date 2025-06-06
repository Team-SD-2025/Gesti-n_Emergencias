import pika
import json
from pymongo import MongoClient

# Conexión a RabbitMQ, usando credenciales simples.
# Nos permite recibir mensajes desde el sistema (como drones o asignación).
rabbitmq_host = "localhost"
credentials = pika.PlainCredentials("monitoreo", "monitoreo123")
parameters = pika.ConnectionParameters(host=rabbitmq_host, credentials=credentials)
connection = pika.BlockingConnection(parameters)
channel = connection.channel()

# Aseguramos que la cola 'registro' esté declarada.
# Aquí llegarán los mensajes de estado de las emergencias.
channel.queue_declare(queue='registro', durable=True)

# Conexión a la base de datos MongoDB local.
# Usamos la base de datos 'emergencias' y su colección del mismo nombre.
mongo = MongoClient("mongodb://localhost:27017/")
db = mongo["emergencias"]
coleccion = db["emergencias"]

# Esta función se ejecuta cada vez que llega un mensaje a la cola.
# Se encarga de registrar o actualizar la información de la emergencia.
def callback(ch, method, properties, body):
    try:
        mensaje = json.loads(body.decode())  # Convertimos el JSON recibido en un diccionario
        print(f"\nMensaje recibido: {mensaje}")

        # Los mensajes pueden venir desde distintos servicios, así que aceptamos dos posibles nombres de campo.
        estado = mensaje.get("status") or mensaje.get("estado")
        nombre = mensaje.get("name") or mensaje.get("ubicacion")
        filtro = {"name": nombre}  # Esto nos permite buscar la emergencia por nombre

        if estado == "En curso":
            # Si la emergencia recién empieza, la registramos completa en MongoDB.
            doc = {
                "emergency_id": mensaje.get("emergency_id"),
                "name": nombre,
                "latitude": mensaje.get("latitude"),
                "longitude": mensaje.get("longitude"),
                "magnitude": mensaje.get("magnitude"),
                "dron_id": mensaje.get("dron_id"),
                "timestamp": mensaje.get("timestamp"),
                "status": "En curso"
            }
            coleccion.insert_one(doc)
            print(f"Emergencia '{nombre}' registrada como EN CURSO.")

        elif estado == "Extinguido":
            # Si la emergencia ya fue resuelta, actualizamos su estado a 'Extinguido'.
            result = coleccion.update_one(
                filtro,
                {"$set": {
                    "status": "Extinguido",
                    "timestamp": mensaje.get("timestamp")
                }}
            )
            if result.modified_count > 0:
                print(f"Emergencia '{nombre}' actualizada a EXTINGUIDO.")
            else:
                print(f"No se encontró emergencia '{nombre}' para actualizar.")

        else:
            # En caso de que llegue un estado inesperado, lo reportamos.
            print(f"Estado no reconocido: {estado}")

        # Indicamos a RabbitMQ que el mensaje fue procesado correctamente.
        ch.basic_ack(delivery_tag=method.delivery_tag)

    except Exception as e:
        # Si ocurre algún error, mostramos el mensaje y rechazamos el mensaje recibido.
        print(f"Error procesando mensaje: {e}")
        ch.basic_nack(delivery_tag=method.delivery_tag)

# El servicio comienza a escuchar la cola 'registro' y usa el callback para procesar cada mensaje entrante.
print("Servicio de REGISTRO escuchando cola 'registro'...\n")
channel.basic_consume(queue='registro', on_message_callback=callback)
channel.start_consuming()
