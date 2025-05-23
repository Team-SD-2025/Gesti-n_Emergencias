import pika
import json
from pymongo import MongoClient

#Se conecta a RabbitMQ en la VM3
rabbitmq_host = "10.10.28.37"
credentials = pika.PlainCredentials("registro", "registro123")
parameters = pika.ConnectionParameters(host=rabbitmq_host, credentials=credentials)
connection = pika.BlockingConnection(parameters)
channel = connection.channel()

channel.queue_declare(queue='registro', durable=True)

#Se conecta a MongoDB en la VM2
mongo = MongoClient("mongodb://localhost:27017/")
db = mongo["emergencias"]
coleccion = db["emergencias"]

def callback(ch, method, properties, body):
    mensaje = json.loads(body.decode())
    print(f"Mensaje recibido: {mensaje}")

    filtro = {"ubicacion": mensaje["ubicacion"], "dron_id": mensaje["dron_id"]}

    if mensaje["estado"] == "En curso":
        coleccion.insert_one({
            "ubicacion": mensaje["ubicacion"],
            "dron_id": mensaje["dron_id"],
            "estado": "En curso",
            "inicio": mensaje["timestamp"]
        })
    elif mensaje["estado"] == "Extinguido":
        coleccion.update_one(
            filtro,
            {"$set": {
                "estado": "Extinguido",
                "fin": mensaje["timestamp"]
            }}
        )

    ch.basic_ack(delivery_tag=method.delivery_tag)


channel.basic_consume(queue='registro', on_message_callback=callback)

print("Servicio de REGISTRO escuchando cola 'registro'...")
channel.start_consuming()
