# Sistema de Gestión de Emergencias con Drones

Este sistema recibe un listado de emergencias, asigna drones para apagar los incendios, mantiene un registro de los eventos ocurridos y entrega al usuario actualizaciones en tiempo real sobre el estado de cada emergencia. Se trata de un sistema distribuido desarrollado principalmente en Go, que utiliza gRPC y RabbitMQ para la asignación de drones y el monitoreo continuo de su estado.


## Integrantes

* **Lorna Mella** - Rol: 202110037-7
* **Diego Mella** - Rol: 202110018-0


## Acceso a máquinas virtuales

Para trabajar en el proyecto `Gestion_Emergencias`, primero debes conectarte a las máquinas virtuales asignadas:

1. Abre una terminal en tu computador.
2. Conéctate al servidor SSH de la universidad:

   ```bash
   ssh nombre_cuenta_DI@ssh2.inf.utfsm.cl
   ```
3. Ingresa tu contraseña de usuario cuando se te solicite.
4. Una vez dentro del servidor `ssh2`, conéctate a la máquina correspondiente con la siguiente información:

| Servicio   | IP          | Contraseña         | Máquina | VM  |
| ---------- | ----------- | ------------------ | ------- | --- |
| Monitoreo  | 10.10.28.35 | `iP6nXsfk7P2AIYGW` | Ubuntu  | VM1 |
| Cliente    | 10.10.28.35 | `iP6nXsfk7P2AIYGW` | Ubuntu  | VM1 |
| Asignación | 10.10.28.36 | `k7ZOgMJKhd28Dru4` | Ubuntu  | VM2 |
| Registro   | 10.10.28.36 | `k7ZOgMJKhd28Dru4` | Ubuntu  | VM2 |
| Drones     | 10.10.28.37 | `je5DReGp7S4Junv3` | Ubuntu  | VM3 |


## Levantar el sistema (por máquina virtual)

> **Nota**: Se asume que los servicios de **RabbitMQ** (en `10.10.28.37:5672`) y **MongoDB** ya están instalados y corriendo en sus respectivas máquinas virtuales. También se asume que las colas de RabbitMQ están creadas y configuradas correctamente, y que la base de datos contiene drones precargados desde `database.mongo`.

### 1. Monitoreo (`monitoreo.go`) – VM1

```bash
ssh nombre_cuenta_DI@ssh2.inf.utfsm.cl
ssh ubuntu@10.10.28.35
cd Gestion_Emergencias/monitoreo
go run monitoreo.go
````

### 2. Asignación (`asignacion.go`) – VM2

```bash
ssh nombre_cuenta_DI@ssh2.inf.utfsm.cl
ssh ubuntu@10.10.28.36
cd Gestion_Emergencias/asignacion
go run asignacion.go
```

### 3. Registro (`registro.py`) – VM2

Este servicio se conecta a la cola `registro` de RabbitMQ y guarda estados de drones.

```bash
ssh nombre_cuenta_DI@ssh2.inf.utfsm.cl
ssh ubuntu@10.10.28.36
cd Gestion_Emergencias/registro
source venv/bin/activate
python3 registro.py
```

> Se asume que el entorno virtual `venv/` ya fue creado y `pika` está instalado (`pip install pika`).

### 4. Drones (`drones.go`) – VM3

```bash
ssh nombre_cuenta_DI@ssh2.inf.utfsm.cl
ssh ubuntu@10.10.28.37
cd Gestion_Emergencias/drones
# MongoDB ya se encuentra corriendo en esta VM (no es necesario volver a ejecutar sudo service mongod start)
# RabbitMQ también ya está activo en esta VM (localhost:5672)
go run drones.go
```

### 5. Cliente (`cliente.go`) – VM1

Debe ejecutarse **al final**, una vez todos los demás servicios estén levantados, ya que depende de ellos:

```bash
ssh nombre_cuenta_DI@ssh2.inf.utfsm.cl
ssh ubuntu@10.10.28.35
cd Gestion_Emergencias/cliente
go run cliente.go
```




## Funcionamiento de la aplicación cliente

Al ejecutar `cliente.go`, el sistema:

1. Lee el archivo `emergencias.json` con una lista de emergencias.
2. Muestra por consola el flujo de atención:

   ```
   Emergencia actual: Incendio Forestal Sur magnitud 5 en x = 40, y = -10
   Se ha asignado dron01 a la emergencia
   Dron en camino a emergencia...
   Dron en camino a emergencia...
   Dron en camino a emergencia...
   .
   .
   .
   Dron apagando emergencia...
   Dron apagando emergencia...
   Dron apagando emergencia...
   .
   .
   .
   Incendio Forestal Sur ha sido extinguido por dron01
   ```
3. Cada mensaje refleja el estado del dron recibido mediante `monitoreo.go` vía RabbitMQ.

## Estructura del proyecto

```
Gestion_Emergencias/
├── asignacion/                 # Servicio de asignación de drones
│   └── asignacion.go
├── cliente/                    # Aplicación cliente (CLI)
│   ├── cliente.go
│   └── emergencias.json        # Emergencias simuladas (5 ejemplos)
├── drones/                     # Servicio de drones (simulación + monitoreo)
│   └── drones.go
├── monitoreo/                  # Servicio de monitoreo vía RabbitMQ + gRPC
│   └── monitoreo.go
├── proto/                      # Definiciones .proto y archivos generados por protoc
│   ├── asignacion.proto
│   ├── asignacion.pb.go
│   ├── asignacion_grpc.pb.go
│   ├── drones.proto
│   ├── drones.pb.go
│   ├── drones_grpc.pb.go
│   ├── monitoreo.proto
│   ├── monitoreo.pb.go
│   └── monitoreo_grpc.pb.go
├── registro/                   # Servicio de registro en Python
│   ├── registro.py
│   └── venv/                   # Entorno virtual Python (para pika)
├── database.mongo              # Archivo con drones precargados
├── go.mod                      # Archivo de dependencias de Go
├── go.sum                      # Checksum de dependencias Go
├── LICENSE
└── README.md

```


## Consideraciones

* **RabbitMQ** usa un exchange `fanout` llamado `monitoreo_exchange` para distribuir actualizaciones.
* **MongoDB** gestiona las ubicaciones de los drones y actualiza su estado.
* **Cliente** muestra mensajes periódicos cada 3 segundos según el estado actual.
* **cliente.go debe ejecutarse al final** para evitar errores por servicios aún no levantados.
* Si se desea probar nuevamente el programa con otro archivo `emergencias.json` se deben ejecutar nuevamente todos los servicios.
* Los datos se basan en coordenadas X/Y y distancias euclidianas.
* Ya se ejecuto `sudo service mongod start` anteriormente, no es necesario repetirlo, a menos que se haya reiniciado la VM.


## Dependencias

* Go (1.20+)
* MongoDB (corriendo en VM3)
* RabbitMQ (corriendo en VM3)
* gRPC + Protocol Buffers
* Python 3.x con `pika` (registro), instalado mediante:

  ```bash
  python3 -m venv venv
  source venv/bin/activate
  pip install pika
  ```


