# Sistema de Gestión de Emergencias con Drones

Este sistema recibe un listado de emergencias, asigna drones para apagar incendios, mantiene un registro de los eventos y entrega actualizaciones en tiempo real al usuario. Se trata de un sistema distribuido desarrollado principalmente en Go, que utiliza **gRPC** y **RabbitMQ** para la asignación de drones y el monitoreo de su estado.

## Integrantes

* **Lorna Mella** - Rol: 202110037-7
* **Diego Mella** - Rol: 202110018-0

## Acceso a máquinas virtuales

Para trabajar en el proyecto `Gestion_Emergencias`, sigue los siguientes pasos:

1. Abre una terminal en tu computador.

2. Conéctate al servidor SSH de la universidad:

   ```bash
   ssh nombre_cuenta_DI@ssh2.inf.utfsm.cl
   ```

3. Ingresa tu contraseña cuando se te solicite.

4. Una vez dentro del servidor `ssh2`, conéctate a la VM correspondiente usando los siguientes datos:

| Servicio   | IP          | Contraseña         | Máquina | VM  |
| ---------- | ----------- | ------------------ | ------- | --- |
| Monitoreo  | 10.10.28.35 | `iP6nXsfk7P2AIYGW` | Ubuntu  | VM1 |
| Cliente    | 10.10.28.35 | `iP6nXsfk7P2AIYGW` | Ubuntu  | VM1 |
| Asignación | 10.10.28.36 | `k7ZOgMJKhd28Dru4` | Ubuntu  | VM2 |
| Registro   | 10.10.28.36 | `k7ZOgMJKhd28Dru4` | Ubuntu  | VM2 |
| Drones     | 10.10.28.37 | `je5DReGp7S4Junv3` | Ubuntu  | VM3 |

## Levantar el sistema (por máquina virtual)

> **Nota**: Los servicios de **MongoDB** y **RabbitMQ** ya están instalados y corriendo en la **VM2**. No es necesario volver a iniciar `mongod` ni configurar RabbitMQ manualmente, ya que las colas (`registro`, `monitoreo`) están predefinidas. La base de datos `emergencias` contiene los drones precargados.

### 1. Monitoreo (`monitoreo.go`) – VM1

Este servicio escucha actualizaciones enviadas desde los drones vía RabbitMQ (`monitoreo`) y las retransmite al cliente por gRPC.

```bash
ssh nombre_cuenta_DI@ssh2.inf.utfsm.cl
ssh ubuntu@10.10.28.35
cd tarea_2/Gestion_Emergencias/monitoreo
go run monitoreo.go
```

### 2. Asignación (`asignacion.go`) – VM2

> **Nota**: Tanto **MongoDB** como **RabbitMQ** están activos localmente en esta máquina:
>
> * MongoDB: `localhost:27017`
> * RabbitMQ: `localhost:5672`

```bash
ssh nombre_cuenta_DI@ssh2.inf.utfsm.cl
ssh ubuntu@10.10.28.36
cd tarea_2/Gestion_Emergencias/asignacion
go run asignacion.go
```

### 3. Registro (`registro.py`) – VM2

Este servicio escucha la cola `registro` de RabbitMQ (también local a esta VM) y guarda los eventos de estado en la base de datos MongoDB (`emergencias`).

```bash
ssh nombre_cuenta_DI@ssh2.inf.utfsm.cl
ssh ubuntu@10.10.28.36
cd tarea_2/Gestion_Emergencias/registro
source venv/bin/activate
python3 registro.py
```

> Considera que `venv/` ya esté creado y `pika` esta instalado:
>
> ```bash
> python3 -m venv venv
> source venv/bin/activate
> pip install pika
> ```

### 4. Drones (`drones.go`) – VM3

Este servicio representa a los drones. Recibe solicitudes de atención y reporta actualizaciones de estado periódicas.

```bash
ssh nombre_cuenta_DI@ssh2.inf.utfsm.cl
ssh ubuntu@10.10.28.37
cd tarea_2/Gestion_Emergencias/drones
go run drones.go
```

### 5. Cliente (`cliente.go`) – VM1

> **Debe ejecutarse al final**, cuando los demás servicios estén corriendo.

```bash
ssh nombre_cuenta_DI@ssh2.inf.utfsm.cl
ssh ubuntu@10.10.28.35
cd tarea_2/Gestion_Emergencias/cliente
go run cliente.go
```

## Funcionamiento de la aplicación cliente

Cuando se ejecuta `cliente.go`, el sistema:

1. Lee un archivo `emergencias.json` con las emergencias a simular.
2. Imprime por consola el proceso de asignación y resolución de emergencias:

   ```
   Emergencia actual: Incendio Forestal Sur magnitud 5 en x = 40, y = -10
   Se ha asignado dron01 a la emergencia
   Dron en camino a emergencia...
   ...
   Dron apagando emergencia...
   ...
   Incendio Forestal Sur ha sido extinguido por dron01
   ```
3. Los mensajes reflejan cambios de estado enviados por `drones.go` a `monitoreo.go` mediante RabbitMQ.

## Estructura del proyecto

```
Gestion_Emergencias/
├── asignacion/                 # Servicio de asignación de drones
│   └── asignacion.go
├── cliente/                    # Cliente de simulación de emergencias
│   ├── cliente.go
│   └── emergencias.json
├── drones/                     # Servicio de dron (simula acciones + reportes)
│   └── drones.go
├── monitoreo/                  # Servicio de monitoreo (fanout → gRPC)
│   └── monitoreo.go
├── proto/                      # Definiciones y compilados .proto
│   ├── asignacion.proto
│   ├── asignacion.pb.go
│   ├── asignacion_grpc.pb.go
│   ├── drones.proto
│   ├── drones.pb.go
│   ├── drones_grpc.pb.go
│   ├── monitoreo.proto
│   ├── monitoreo.pb.go
│   └── monitoreo_grpc.pb.go
├── registro/                   # Servicio de registro de eventos
│   ├── registro.py
│   └── venv/
├── database.mongo              # Script para poblar colección `drones`
├── go.mod
├── go.sum
└── README.md
```

## Consideraciones

* MongoDB y RabbitMQ **corren en VM2**, no es necesario iniciarlos manualmente salvo que se haya reiniciado la máquina.
* * MongoDB se encarga de:
  * Gestionar la **ubicación actual** de cada dron.
  * **Actualizar la posición del dron** cada vez que este atiende una emergencia, quedando registrado su nuevo punto final.
  * Guardar el estado de cada emergencia (En curso, Extinguido) en la colección `emergencias`.
* Todos los servicios deben estar activos antes de ejecutar `cliente.go`, de lo contrario fallará la conexión.
* Las distancias entre drones y emergencias se calculan usando **distancia euclidiana** en coordenadas (X,Y).
* Para volver a simular con nuevas emergencias, se debe reiniciar el sistema completo, ejecutando nuevamente todos los servicios.


## Dependencias

* Go (1.20 o superior)
* MongoDB (instalado y corriendo en VM2)
* RabbitMQ (instalado y corriendo en VM2)
* gRPC + Protocol Buffers
* Python 3.x (solo para `registro.py`)
* Librería `pika` para Python:

  ```bash
  python3 -m venv venv
  source venv/bin/activate
  pip install pika
  ```
