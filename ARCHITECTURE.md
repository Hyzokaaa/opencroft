# Arquitectura

OpenCroft es un plano de control para servidores basados en contenedores de sistema
(LXD o Incus): contenedores aislados, dominios, certificados y — en fases posteriores —
despliegue de aplicaciones.

Un único binario que funciona como CLI y como daemon HTTP, con la UI embebida.

## Principio rector

> **La fuente de verdad es el sistema operativo, no la base de datos de OpenCroft.**

LXD sabe qué contenedores existen. nginx sabe qué rutas existen. Los ficheros de
certificado dicen cuándo caducan. OpenCroft **lee** ese estado y **escribe** sobre él, pero nunca lo posee.

Consecuencias que atraviesan todo el diseño:

- Si borras la base de datos de OpenCroft, no pierdes infraestructura. Al arrancar la
  redescubre entera.
- Si creas un contenedor a mano con `lxc launch`, OpenCroft lo ve y lo lista.
- Si editas un vhost de nginx con vim, OpenCroft detecta el cambio y **no** lo pisa.
- Todo lo que hace la UI existe como comando CLI, y es el mismo código.

La base de datos guarda únicamente lo que **no se puede derivar del sistema**: usuarios,
sesiones, tokens, log de auditoría, cola de trabajos y ajustes. Nada más.

## Stack

| Capa | Tecnología | Motivo |
|---|---|---|
| Daemon + CLI | Go | Binario estático, sin runtime en el host, instalable con un `curl` |
| UI | React + Vite + Tailwind | Embebida en el binario vía `go:embed` |
| Estado propio | SQLite | Fichero único, sin servidor de base de datos que administrar |
| Infraestructura | LXD o Incus, nginx | Ya instalados en el host, gestionados vía sus propias interfaces |
| TLS | ACME embebido (`lego`) | Sin certbot ni plugins por proveedor DNS |

Es el stack estándar de los planos de control de infraestructura (Portainer, Rancher,
Traefik, Nomad, Caddy). La razón es operativa, no estética: el usuario objetivo es un
sysadmin que no quiere instalar Node en su servidor de producción.

## Estructura de módulos

Misma separación por capas que el resto de proyectos: dominio, aplicación e
infraestructura. En Go los paquetes sustituyen a los módulos de Nest, y las interfaces
sustituyen a la inyección por decoradores.

```
cmd/
  croft/                          # entrypoint único (CLI + daemon)

internal/
  <module>/
    application/
      commands/
      queries/
    domain/
      entities/
      enums/
      repositories/               # interfaces
      services/                   # lógica de negocio
    infrastructure/
      http/                       # handlers HTTP  (equivale a nest/controllers)
      cli/                        # comandos cobra (equivale a nest/controllers)
      lxd/                        # driver de runtime: LXD
      incus/                      # driver de runtime: Incus
      nginx/                      # driver de proxy sobre ficheros de nginx
      acme/                       # emisión de certificados (lego)
      sqlite/                     # repositorios sobre SQLite
  shared/
    id/                           # value object Id (ULID)
    host/                         # abstracción de ejecución (local / remoto)
    reconcile/                    # motor de detección de drift

web/                              # React + Vite + Tailwind
```

Módulos previstos: `instance`, `route`, `certificate`, `snapshot`, `host`, `auth`,
`audit`, `job`. En fase PaaS se añaden `app`, `build` y `env`.

## Capas y responsabilidades

### Entidades (domain/entities)

Estructuras de datos puras. Sin lógica de negocio. Campos públicos. Se construyen
mediante `Props`. Usan `Id` como value object (ULID) para identificadores.

```go
type Instance struct {
    Id       shared.Id
    Name     string
    OS       string
    IP       string
    CPULimit int
    MemLimit string
    Status   InstanceStatus
    Managed  bool // false si existe en LXD pero OpenCroft no lo creó
}

func NewInstance(props InstanceProps) *Instance {
    return &Instance{
        Id:       shared.NewId(props.Id),
        Name:     props.Name,
        OS:       props.OS,
        IP:       props.IP,
        CPULimit: props.CPULimit,
        MemLimit: props.MemLimit,
        Status:   props.Status,
        Managed:  props.Managed,
    }
}

func (i *Instance) GetId() string { return i.Id.Get() } // solo para value objects
```

### Domain Services (domain/services)

Toda la lógica de negocio vive aquí. Validan reglas, crean entidades, orquestan
operaciones. Dependen de abstracciones (interfaces de repositorio). Método principal:
`Execute()`.

- **Fichero**: `<entidad>-<acción>.go` (ej. `instance-create.go`)
- **Tipo**: acción + entidad en PascalCase, sin sufijo `Service` (ej. `CreateInstance`)

```go
type CreateInstance struct {
    idGenerator IdGenerator
    instances   InstanceRepository
    networks    NetworkRepository
}

func NewCreateInstance(
    idGenerator IdGenerator,
    instances InstanceRepository,
    networks NetworkRepository,
) *CreateInstance {
    return &CreateInstance{idGenerator, instances, networks}
}

func (s *CreateInstance) Execute(ctx context.Context, props CreateInstanceProps) (*Instance, error) {
    if existing, _ := s.instances.FindByName(ctx, props.Name); existing != nil {
        return nil, ErrInstanceAlreadyExists
    }

    ip, err := s.networks.AllocateAddress(ctx)
    if err != nil {
        return nil, err
    }

    instance := NewInstance(InstanceProps{
        Id:       s.idGenerator.Create(),
        Name:     props.Name,
        OS:       props.OS,
        IP:       ip,
        CPULimit: props.CPULimit,
        MemLimit: props.MemLimit,
        Status:   StatusRunning,
        Managed:  true,
    })

    if err := s.instances.Create(ctx, instance); err != nil {
        return nil, err
    }
    return instance, nil
}
```

### Commands y Queries (application/)

Casos de uso. Delegan en Domain Services, nunca contienen lógica de negocio. Reciben
dependencias por constructor normal — no hay framework de inyección, igual que la
convención de no usar `@Injectable()` en los Commands.

```go
type CreateInstanceCommand struct {
    createInstance *services.CreateInstance
    events         InstanceEventEmitter
}

func (c *CreateInstanceCommand) Execute(ctx context.Context, props Props) (CreateInstanceResponse, error) {
    instance, err := c.createInstance.Execute(ctx, services.CreateInstanceProps{
        Name:     props.Request.Name,
        OS:       props.Request.OS,
        CPULimit: props.Request.CPULimit,
        MemLimit: props.Request.MemLimit,
    })
    if err != nil {
        return CreateInstanceResponse{}, err
    }

    c.events.EmitInstanceCreated(ctx, InstanceCreatedEvent{Id: instance.GetId()})
    return CreateInstanceResponse{Id: instance.GetId()}, nil
}
```

### Handlers HTTP y CLI (infrastructure/)

Entrada/salida. Instancian implementaciones concretas y construyen los Commands/Queries
a mano, exactamente como hacen los controllers de Nest.

**Aquí está la clave de "manual y automático a la vez"**: el handler HTTP y el comando
CLI son dos entradas distintas al *mismo* Command. La UI no reimplementa nada.

```go
// infrastructure/http/instance-handler.go
func (h *InstanceHandler) Create(w http.ResponseWriter, r *http.Request) {
    var body CreateInstanceRequest
    // ... decode

    service := services.NewCreateInstance(h.idGenerator, h.instances, h.networks)
    command := commands.NewCreateInstanceCommand(service, h.events)
    res, err := command.Execute(r.Context(), commands.Props{Request: body})
    // ... respond
}

// infrastructure/cli/instance-create.go
func newInstanceCreateCmd(deps *Deps) *cobra.Command {
    return &cobra.Command{
        Use: "create <name>",
        RunE: func(cmd *cobra.Command, args []string) error {
            service := services.NewCreateInstance(deps.IdGenerator, deps.Instances, deps.Networks)
            command := commands.NewCreateInstanceCommand(service, deps.Events)
            res, err := command.Execute(cmd.Context(), commands.Props{Request: buildRequest(args, cmd.Flags())})
            // ... print
            return err
        },
    }
}
```

### Repositorios

Interfaz en dominio, implementación en infraestructura. Devuelven `nil` si no encuentran
la entidad.

La diferencia con un backend convencional: **el repositorio no habla con una base de
datos, habla con el sistema operativo.** Esto es lo que hace que la arquitectura por
capas encaje aquí de forma natural — la inversión de dependencias ya estaba pensada para
esto.

```go
// domain/repositories/instance.repository.go
type InstanceRepository interface {
    Create(ctx context.Context, instance *Instance) error
    FindByName(ctx context.Context, name string) (*Instance, error)
    FindAll(ctx context.Context) ([]*Instance, error)
    Delete(ctx context.Context, name string) error
}

// infrastructure/lxd/lxd-instance.repository.go
type LXDInstanceRepository struct {
    host host.Host
}

func (r *LXDInstanceRepository) FindAll(ctx context.Context) ([]*Instance, error) {
    // lee de LXD, la fuente de verdad — no de una tabla
}
```

Conviven dos familias de implementaciones y la distinción es deliberada:

| Repositorio | Implementación | Fuente de verdad |
|---|---|---|
| `InstanceRepository` | `LXDInstanceRepository` | LXD |
| `RouteRepository` | `NginxRouteRepository` | ficheros de nginx |
| `CertificateRepository` | `FileCertificateRepository` | los propios ficheros PEM |
| `UserRepository` | `SQLiteUserRepository` | SQLite (dato propio de OpenCroft) |
| `job.Journal` | `JobJournal` | SQLite (lo que OpenCroft hizo — dato propio) |

Si un repositorio de infraestructura necesita SQLite, es señal de alarma: significa que
OpenCroft está intentando poseer estado que pertenece al sistema.

### Event Emitters

Dependencias de infraestructura que se pasan al Command desde el handler. Ubicados en
`infrastructure/events/`. Alimentan el log de auditoría, el streaming de logs a la UI y
los webhooks.

## Abstracción de Host

Toda ejecución contra el sistema pasa por `shared/host`. Ninguna capa llama a `exec` ni
a `lxc` directamente.

```go
type Host interface {
    Run(ctx context.Context, cmd string, args ...string) (Output, error)
    ReadFile(ctx context.Context, path string) ([]byte, error)
    WriteFile(ctx context.Context, path string, content []byte, mode os.FileMode) error
    RemoveFile(ctx context.Context, path string) error
}
```

Hoy existe una única implementación, `LocalHost`. El día que se quiera gestionar varios
servidores se añade `RemoteHost` y no cambia una línea de dominio ni de aplicación.

Por eso la API se diseña con el host como parámetro desde el principio, aunque solo haya
uno:

```
GET  /api/hosts/{hostId}/instances
POST /api/hosts/{hostId}/instances
```

Con `hostId = "local"` por defecto. Es coste cero ahora y evita un refactor completo
después.

## Portabilidad entre distribuciones

Coolify funciona en cualquier Linux porque lo mete todo en Docker: su única dependencia es
"un Linux con Docker". Es elegante, y es precisamente la dependencia que no queremos. Sin
ese truco no se alcanza el mismo grado de independencia — pero sí se puede reducir el
acoplamiento a casi nada.

El objetivo realista es **cualquier Linux con LXD o Incus**: Debian, Ubuntu, Fedora, Arch,
Alpine, openSUSE. Se consigue con tres decisiones.

### ACME embebido, sin certbot

certbot es Python, su empaquetado varía por distribución y cada proveedor DNS es un
paquete aparte (`python3-certbot-dns-ovh` y equivalentes). La biblioteca `lego` trae los
proveedores dentro y compila en el binario.

Elimina una dependencia del host, elimina el empaquetado por proveedor, y elimina el
"instala este plugin" del manual de instalación. Es la mayor ganancia de las tres.

Los certificados que ya gestione certbot en el host se siguen leyendo y mostrando: se
adoptan como cualquier otro recurso `unmanaged`.

**Un solo dueño por dominio.** Un dominio que sirve un certificado de certbot tiene dos
programas a cargo: croft escribe el vhost y certbot renueva con su propia configuración
de DNS, que nadie recuerda hasta que el dominio cambia de proveedor y la renovación
falla en silencio. *Issue with croft* lo resuelve en un paso: croft emite el suyo, reescribe
el vhost para servirlo y, solo después, mueve el fichero de renovación de certbot a
`/var/lib/croft/taken-over/`. No se borra nada: los ficheros de certbot siguen ahí y
moverlo de vuelta lo deshace. Si otro vhost todavía lee ese certificado, certbot sigue
renovándolo.

**Comodines.** Con credenciales DNS, croft pide un certificado para `*.example.com` y
`example.com` juntos y lo guarda en `/var/lib/croft/certificates/_.example.com` — `_` no
puede empezar un nombre de host, así que nunca choca con el de un subdominio. Desde ese
momento *Enable https* en `app.example.com` no pide nada a Let's Encrypt: el vhost apunta
al comodín. Cubre un nivel, como lo lee la autoridad: `app.example.com` sí,
`api.app.example.com` no. Se renueva como cualquier otro, siempre por DNS.

### Driver de runtime: LXD e Incus

Incus es el fork que crearon los desarrolladores originales de LXD, con la misma API y
empaquetado nativo en Debian, Ubuntu, Alpine, Arch y Fedora — sin snap, que es incómodo
fuera del mundo Ubuntu.

```go
type ContainerRuntime interface {
    List(ctx context.Context) ([]*Instance, error)
    Create(ctx context.Context, spec InstanceSpec) error
    Annotate(ctx context.Context, name, key, value string) error
    // ...
}
```

Dos implementaciones, `LXDRuntime` e `IncusRuntime`, detectadas al arrancar. El dominio no
sabe cuál está usando.

### Driver de proxy, sin asumir el layout de Debian

`/etc/nginx/sites-available` y `sites-enabled` son una convención de Debian y Ubuntu.
Fedora, RHEL, Alpine y Arch usan `/etc/nginx/conf.d/*.conf` y no tienen esos directorios.
Cualquier herramienta que los dé por sentados solo funciona en media familia de distros.

La solución es además más limpia que el baile de symlinks: **un directorio propio y una
sola línea gestionada** en la configuración de nginx.

```nginx
# /etc/nginx/nginx.conf — una única línea añadida por OpenCroft
include /etc/nginx/croft.d/*.conf;
```

Todos los vhosts generados viven en `/etc/nginx/croft.d/`. Funciona igual en cualquier
distribución, y desinstalar OpenCroft es borrar un directorio y una línea.

```go
type Proxy interface {
    Routes(ctx context.Context) ([]*Route, error)
    Write(ctx context.Context, route *Route) error
    Remove(ctx context.Context, domain string) error
    Reload(ctx context.Context) error
}
```

Con `NginxProxy` como única implementación al principio. Un `CaddyProxy` posterior encaja
sin tocar nada más — y Caddy resuelve TLS por su cuenta, lo que lo hace atractivo para
quien no quiera gestionar certificados.

### Lo que no se abstrae

**systemd.** Todo lo que no sea Alpine lo usa, y abstraer el gestor de servicios por un
caso minoritario no compensa. Si algún día importa, es otro driver más.

## Modelo de recursos

```
Host
 |
 +-- Instance          contenedor LXC
 |    +-- Snapshot     copia puntual (lxc snapshot)
 |    +-- App          [fase PaaS] código desplegado dentro de la Instance
 |
 +-- Route             dominio -> destino (vhost de nginx)
      +-- Certificate  certificado TLS de la ruta
```

Decisión importante para poder escalar a PaaS: **`App` no es un tipo paralelo a
`Instance`, es una capa encima.** Una App es una Instance con origen de código, receta de
build, servicio de runtime y variables de entorno. Esto significa que:

- Todo lo que se construya en la v1 sobre `Instance` sigue sirviendo en la fase PaaS.
- Un usuario puede empezar con un contenedor a mano y "promoverlo" a App después.
- No hay dos caminos de código que mantener en paralelo.

Es exactamente lo que evita el error de Coolify, donde un recurso gestionado y uno no
gestionado son mundos separados.

## Motor de reconciliación

Es el corazón del producto y lo que lo diferencia. Vive en `shared/reconcile` y se apoya
en dos conceptos:

- **Estado observado** — lo que el sistema dice ahora mismo (`lxc list`, `nginx -T`,
  los ficheros de certificado en disco).
- **Estado deseado** — lo que OpenCroft cree que debería haber, derivado de las anotaciones
  del propio recurso.

La diferencia entre ambos es **drift**, y drift no es un error: es información que se
muestra al usuario.

```
Observado  --.
              >--  Diff  -->  Drift  -->  se muestra, nunca se aplica solo
Deseado    --'
```

Reglas duras:

1. **OpenCroft nunca aplica cambios que el usuario no ha pedido.** Detectar drift produce un
   aviso, no una corrección automática.
2. **Toda escritura tiene modo plan.** `--plan` en CLI y una pantalla de confirmación en
   la UI muestran los comandos exactos y el diff de cada fichero antes de tocar nada.
3. **Nada de `--force` implícito.**

### Estado deseado sin base de datos

El estado deseado se guarda **en el propio recurso**, no en SQLite. LXD admite metadatos
de usuario arbitrarios:

```bash
lxc config set miapp user.croft.managed=true
lxc config set miapp user.croft.domain=miapp.example.com
lxc config set miapp user.croft.port=3000
```

Si borras la base de datos de OpenCroft, esto sobrevive. Si migras el contenedor a otro
servidor, viaja con él.

### Ficheros generados y edición manual

Cada fichero que genera OpenCroft lleva una cabecera con el hash del contenido generado:

```nginx
# managed-by: croft
# croft-hash: 8f14e45fceea167a5a36dedd4bea2543
# ¿Editado a mano? OpenCroft lo detectará y dejará de gestionarlo automáticamente.
server {
    ...
}
```

Al reconciliar, OpenCroft recalcula el hash del fichero en disco:

- **Coincide** → el fichero es suyo, puede regenerarlo con seguridad.
- **No coincide** → alguien lo editó a mano. OpenCroft marca la ruta como `adopted`, muestra
  el diff, y **deja de sobreescribirla**. Sigue mostrándola en la UI, sigue avisando si el
  certificado caduca, pero no la toca.
- **Sin cabecera** → fichero ajeno. Aparece como `unmanaged` y se puede *tomar*
  explícitamente (ver *Tomar un vhost escrito a mano*).

Este es el mecanismo concreto que hace real la promesa de "automatiza lo que quieras pero
permite usarlo a mano". No es una política, es código.

### Rutas por prefijo y websockets

Una ruta es un dominio con un destino para `/` y, opcionalmente, **paths**: prefijos que
van a otro sitio. La web en `/`, su API en `/api/`, en contenedores distintos o en el
mismo con otro puerto — sin un segundo nginx dentro del contenedor para repartirlos.

```nginx
location / {
    proxy_pass http://10.146.38.200:80;
    ...
}
location /api/ {
    proxy_pass http://10.146.38.201:3000/;   # "Strip": el backend recibe /users, no /api/users
    ...
}
```

- Un prefijo empieza y acaba en `/` (`/api/` no se traga `/apiary`) y ningún segmento es
  solo puntos. El agente lo valida otra vez: el panel no es de fiar para escribir nginx.
- **Strip** quita el prefijo antes de pasar la petición; sin él llega entero.
- **Toda** `location` que escribe croft deja pasar un websocket: `proxy_http_version 1.1`,
  `Upgrade $http_upgrade`, `Connection $http_connection` y un `proxy_read_timeout` de una
  hora. No es una opción que haya que acordarse de marcar: una ruta que corta un websocket
  es un fallo que solo se ve en producción.
- Los paths se leen de vuelta del fichero, igual que el resto: el vhost es la fuente de
  verdad, no la base de datos.

Solo se añaden paths a un dominio que croft gestiona. Uno `adopted` (editado a mano después)
se respeta; uno `unmanaged` hay que tomarlo antes.

### Tomar un vhost escrito a mano

Un vhost que existía antes de croft —típicamente el que dejó certbot— aparece como
`unmanaged` y croft no lo toca. *Take over* lo convierte en suyo sin que el visitante note
nada:

1. Lee del fichero lo que importa: destino, puerto, si va por https y **dónde está su
   certificado** (`/etc/letsencrypt/live/...` o el directorio de croft; otro sitio se
   rechaza).
2. Mueve el original a `/var/lib/croft/taken-over/<fichero>.<fecha>`. No se borra nada.
3. Escribe el vhost de croft y pasa `nginx -t`. Si nginx lo rechaza, el fichero de croft
   se va y **el original vuelve a su sitio**: nginx queda como estaba, no sin ninguno.

Se niega si el fichero sirve también otros dominios (se los llevaría por delante), si es
`nginx.conf` o si no tiene un destino que croft sepa leer. Lo que el original tuviera de
más —un `location` con reglas propias— no se copia: por eso se ve el plan antes.

### Proyectos

Un proyecto son los contenedores que van juntos —la web, el backend y su base de datos— y,
a través de ellos, los dominios que les llegan. No hay tabla de proyectos en el panel:

- **La pertenencia** es una anotación del contenedor, `user.croft.project`. Viaja con él
  al migrarlo, como el resto de su estado deseado. Mover un contenedor de proyecto es un
  `lxc config set`: no reinicia nada ni copia nada.
- **La declaración** es un fichero en el host, `/etc/croft/projects/<nombre>.conf`, con su
  descripción. Es lo que permite que un proyecto exista antes de su primer contenedor. Vive
  en `/etc` como `dns.conf`, así que perder la base de datos del panel no lo pierde.
- **Lo que se muestra es la unión de los dos.** Un proyecto declarado sin contenedores está
  esperando el primero. Una anotación que nombra un proyecto no declarado aquí —un
  contenedor migrado de otro host— aparece como *found, not declared*, y declararlo es un
  paso. Es el mismo trato que reciben un vhost o una unidad que croft no creó.
- **Nada queda huérfano.** Un proyecto con contenedores no se borra: hay que sacarlos antes,
  a propósito. Y solo se mueve un contenedor a un proyecto declarado, para que una errata no
  invente uno.

Los dominios no se asignan: pertenecen al proyecto de los contenedores a los que apuntan,
incluidos los paths. Los snapshots siguen siendo por contenedor; un proyecto agrupa, no
sincroniza.

### Adopción

OpenCroft descubre lo que no ha creado él: contenedores lanzados con `lxc launch`, vhosts
escritos a mano, certificados emitidos por otro medio. Todos aparecen listados como
`unmanaged`. Adoptarlos es una operación explícita que solo añade anotaciones — no
reescribe nada.

Dentro de un contenedor, lo mismo vale para lo que corre en él. Una instalación hecha a
mano —un `install.sh` que deja un backend en systemd y un frontend compilado en
`/var/www`— aparece en el panel como *found, not created by croft*, con dos reglas de
descubrimiento que leen el sistema en vez de adivinar:

- **Unidades**: un `*.service` escrito directamente en `/etc/systemd/system/` es algo que
  una persona o un script puso ahí. Los de la distribución viven en `/lib` o `/usr/lib`, y
  los que un paquete enlaza en `/etc/systemd/system` (syslog, dbus, open-vm-tools) son
  symlinks — se excluyen: se apuntaron ahí, no se escribieron. `croft-*` también se
  excluye: eso ya es un servicio nuestro.
- **Sitios**: los bloques `server` del nginx *de dentro* del contenedor (`nginx -T`) que
  sirven un directorio con `root`. El catch-all `_` de la distribución no es un sitio de
  nadie.

Unidades y sitios salen de **una sola ejecución** dentro del contenedor, y el estado de
todas las unidades de **una sola pregunta** a `systemctl`: leer un contenedor cuesta lo
mismo tenga lo que tenga.

Un sitio publicado no dice de dónde viene — los ficheros servidos son un build, y el código
vive en otro sitio. OpenCroft **se lo pregunta a los ficheros**: el checkout cuyo
`dist/index.html` es idéntico byte a byte al `index.html` servido es el que lo produjo.
Eso es una prueba, no una conjetura; si nada coincide, lo dice y no elige el directorio más
probable.

Adoptar es un trato con límites claros. OpenCroft **toma el ciclo de vida del código**
—fetch, build, restart o publicación, snapshot, rollback— y **deja como estaba lo que otro
escribió**: la unidad (con su `User=`, sus `After=`) y la configuración de nginx. El `.env`
solo cambia cuando se edita desde el panel, y entonces solo en las líneas que se tocaron
(ver *El entorno vive en su archivo*). Es el mismo contrato que con un vhost editado a mano. Todo lo que se registra se lee del
contenedor —directorio, usuario, rama, repositorio— y nunca de la petición: un navegador no
puede declarar que algo está adoptado para elegir qué unidad reinicia croft o a quién le
entrega los ficheros. Soltar un servicio adoptado (*Release*) olvida las anotaciones y
nada más, porque croft no lo puso ahí.

### `croft explain`

Cada operación sabe imprimir su equivalente manual:

```
$ croft instance create miapp --os ubuntu:24.04 --explain

lxc launch ubuntu:24.04 miapp
lxc config set miapp limits.cpu 4
lxc config set miapp limits.memory 4GB
lxc config device set miapp eth0 ipv4.address 10.146.38.20
lxc config set miapp user.croft.managed=true
```

La UI muestra lo mismo en un panel plegable. El usuario nunca queda encerrado dentro de la
herramienta y puede aprender el sistema en lugar de depender del panel.

## Red y direccionamiento

Problema actual de los scripts: `proxy_pass http://$CONTAINER_IP` congela una IP asignada
por DHCP. Si el contenedor reinicia y cambia de IP, el proxy apunta a la nada y nada lo
detecta.

Solución: **OpenCroft asigna direcciones estáticas** dentro del rango de la red LXD,
reservando el tramo alto para evitar colisiones con el DHCP de LXD.

```bash
lxc config device set <instancia> eth0 ipv4.address 10.146.38.20
```

Además, la reconciliación valida en cada pasada que el `proxy_pass` de cada ruta apunta a
la IP real de la instancia, y avisa si no coincide. El puerto interno es configurable
(`user.croft.port`), no fijo a 80 como ahora.

## Trabajos y streaming

Crear un contenedor, emitir un certificado o construir una imagen tardan minutos. No
pueden vivir en una petición HTTP.

Toda operación larga corre como `Job` en el daemon, con su progreso en streaming por SSE.
Si cierras el navegador, el trabajo sigue.

**Un trabajo a la vez por contenedor.** Dos despliegues en el mismo contenedor a la vez
chocan: un snapshot congela el contenedor y el siguiente comando del otro falla con
*"Instance is frozen"*; y aunque no, los dos pelearían por el lock de apt y cada uno
fotografiaría al otro a medias. El agente da turnos por contenedor: el segundo espera, y lo
dice en su progreso. Trabajos en contenedores distintos siguen corriendo a la vez — por eso
separar recursos en contenedores es la forma de desplegarlos en paralelo.

Cada trabajo **se escribe en SQLite al empezar y otra vez al terminar**, con su plan, lo
que narró cada paso y el error si falló. Es lo único que el sistema no puede devolver: un
contenedor dice qué corre ahora, no que el despliegue anterior falló en su build. La vista
*Activity* del panel lo lee — cada despliegue con su resultado, hasta qué paso llegó y por
qué se detuvo.

- Al arrancar, lo que quedó como `running` de una ejecución anterior del daemon pasa a
  `interrupted`: se abandonó, no terminó, y decir otra cosa sería mentir.
- Un paso marcado `Secret` (el que escribe el `.env`) se muestra antes de correr, como
  todos, pero **no se escribe después**: ni su comando ni el error que lo cita. Del resto
  se guarda todo, porque el comando exacto y la salida de un paso fallido son justo lo que
  hace falta para saber por qué falló.
- Se conservan los últimos mil. Borrar la base cuesta el historial — nunca la
  infraestructura.

## Seguridad

OpenCroft necesita root para hablar con LXD o Incus, escribir en `/etc/nginx` y leer los
certificados.
Darle root a un proceso que además sirve HTTP a internet es exactamente el fallo de diseño
que arrastra Portainer.

**Separación en dos procesos:**

```
  navegador
     |  HTTPS
  croft-api        usuario sin privilegios, sirve UI y API, valida y autentica
     |  unix socket, /run/croft.sock
  croft-agent      root, superficie mínima, solo ejecuta operaciones ya validadas
     |
  LXD o Incus / nginx / ficheros TLS
```

`croft-agent` expone un conjunto cerrado de operaciones tipadas. No acepta cadenas de
comando arbitrarias — no existe un endpoint "ejecuta esto".

Otras decisiones:

- Autenticación local con sesiones, tokens de API para automatización, OIDC opcional más
  adelante.
- Historial de toda operación de escritura: qué, sobre qué, cuándo y cómo terminó (ver
  *Trabajos y streaming*). Quién y desde dónde llegan con los equipos (fase 5).
- Los ficheros de metadatos nunca se leen con `source` (los scripts actuales lo hacen, y
  como corren con root eso es ejecución de código arbitrario). Formato parseado, no
  ejecutado.

## Testing

Igual que en el resto de proyectos: mocks que implementan la interfaz real, operando en
memoria.

Aquí esto rinde especialmente bien, porque permite probar toda la lógica de negocio
—incluida la reconciliación y la detección de drift— sin LXD, sin nginx y sin root.

```go
instances := mocks.NewMockInstanceRepository()
networks  := mocks.NewMockNetworkRepository()
ids       := mocks.NewFakeIdGenerator("test-id")

service := services.NewCreateInstance(ids, instances, networks)
result, err := service.Execute(ctx, services.CreateInstanceProps{
    Name: "miapp",
    OS:   "ubuntu:24.04",
})
```

Para la capa de infraestructura, un `FakeHost` que registra los comandos ejecutados y
devuelve salidas preparadas. Así se puede verificar que `CreateInstance` produce
exactamente la secuencia de comandos esperada, sin ejecutar ninguno.

Y un tercer nivel que costó aprenderlo: **el puente entre los dos procesos**. El panel
registra cada ruta a mano y reenvía al agente, y cada mitad probada por separado pasaba sus
tests mientras un botón del panel no llegaba a ningún sitio (restart salió así en v0.21.0).
Los tests del paquete `server` recorren cada acción desde la ruta del panel hasta el
agente, con un agente falso que registra lo que se le pidió. Las acciones de un conjunto
cerrado —restart, stop, start— se registran en ambos lados desde la misma lista, para que
no puedan desincronizarse.

## Módulo `deploy`

Un contenedor corre **servicios**, en plural. Un servicio es un origen en git, unos
comandos para construirlo y uno para arrancarlo. No es una entidad nueva junto a
`Instance`: es algo que una instancia contiene.

```
internal/deploy/
  domain/
    entities/      service.go, snapshot.go — incluida la Adoption
    services/      detect.go — propone, nunca decide
  infrastructure/
    container/     planner.go — convierte un Service en comandos

internal/agent/
  units.go         descubrir unidades y sitios; restart, stop, start de cualquier unidad
  adopt.go         inspeccionar y adoptar una unidad
  adopt_site.go    inspeccionar y adoptar un sitio, rastreando el checkout que lo construyó
```

### Desplegar son dos planes, no uno

No se puede saber cómo construir código que no se ha visto. Así que el primer plan dice
*"tráelo para que pueda mirarlo"* — snapshot, herramientas, checkout — y el segundo dice
exactamente qué significa construirlo y arrancarlo. Ambos se leen antes de que corra
ninguno.

El plan de inspección es **el prefijo del de despliegue**, construido por el mismo código.
Si se escribieran aparte, aprobar el primero dejaría de decir nada sobre el segundo.

La detección lee `package.json`, `go.mod`, `index.html` y propone comandos concretos, que
llegan a la interfaz como cajas de texto editables y se guardan como anotaciones. Es lo
contrario de un buildpack: adivinamos igual que cualquiera, pero a la vista.

### Anotaciones

```
user.croft.services                        "backend client"  ← el índice
user.croft.service.<nombre>.repo           origen
user.croft.service.<nombre>.commit         lo desplegado ahora mismo
user.croft.service.<nombre>.install|build|start
user.croft.service.<nombre>.health         ruta, código, texto esperado
user.croft.service.<nombre>.healthy        el snapshot que pasó su comprobación

# solo si se adoptó en vez de desplegarse
user.croft.service.<nombre>.adopted-unit   la unidad que ya tenía, con su nombre
user.croft.service.<nombre>.adopted-site   el directorio que ya servía su nginx
user.croft.service.<nombre>.output         dónde deja el build los ficheros (un sitio)
user.croft.service.<nombre>.run-as         dueño del checkout, al que se le devuelve
user.croft.service.<nombre>.env-file       su entorno — se muestra, nunca se escribe
```

Sin el índice no hay forma de enumerar: solo de preguntar por nombres ya conocidos. La
lista de claves la da una sola función (`Service.Record`), la misma para lo desplegado y
lo adoptado, y la configuración **solo se registra cuando un despliegue termina bien**: un
despliegue que falla no deja escrita la configuración que lo rompió.

### El entorno vive en su archivo

Las variables de un servicio están en **un solo sitio**: el archivo que lee su unidad al
arrancar (`<path>/.env` para lo desplegado por croft, el `EnvironmentFile=` de una unidad
adoptada) o, para un sitio, el que lee su build. No hay copia en las anotaciones — hasta la
0.23 la había, como JSON, y se borra en el siguiente despliegue: era un segundo lugar para
los secretos y una verdad que se separaba del archivo en cuanto alguien lo editaba.

El panel es un editor de ese archivo:

- **Cambia solo lo que se tocó.** Comentarios, líneas en blanco, orden, un `export` delante,
  las comillas que alguien eligió: todo queda igual. Una variable nueva va al final.
- **Nunca pisa otra edición.** Lo que se lee lleva el hash del archivo, y el cambio se
  rechaza si el archivo ya no tiene exactamente eso — comprobado otra vez en el último
  momento, ya dentro del turno del contenedor. Una edición hecha por ssh mientras el panel
  estaba abierto no se pierde.
- **Se escribe por base64, no con un heredoc.** El contenido es lo que el archivo ya tenía,
  y una línea que coincidiera con el marcador de cierre del heredoc lo cortaría y ejecutaría
  el resto como root: un servicio capaz de escribir su propio `.env` podría así ejecutar
  cualquier cosa la próxima vez que alguien lo editara desde el panel. Escribir *dentro* del
  archivo conserva su dueño y sus permisos; uno nuevo nace legible solo por su dueño.
- **Se aplica como lo lee cada uno:** un proceso, reiniciando su unidad; un sitio,
  reconstruyendo y publicando. Con snapshot antes, como todo cambio a un servicio.

El primer despliegue escribe el entorno inicial que se le da. Después, un despliegue
**nunca** lo reescribe: el archivo manda.

### Redesplegar

*Redeploy* es el despliegue de todos los días: lo que el contenedor ya tiene anotado
—repositorio, rama, comandos, entorno— sin formulario y sin que la petición lleve nada. Se
salta el formulario, no el plan: se muestra y se aprueba igual. Sigue la rama: el commit
anotado dice qué se desplegó la última vez, no qué desplegar ahora. Cambiar rama, comandos
o entorno es *Properties*, que es el mismo despliegue con el formulario delante.

### Servicios adoptados y sitios

Un servicio adoptado se despliega donde ya estaba: se hace fetch del checkout existente
(nunca se vuelve a clonar ni se vuelve shallow, con `safe.directory` solo para ese
comando, porque suele ser de otro usuario), se construye, se le devuelven los ficheros a
su dueño y se reinicia **su** unidad. Un despliegue no escribe unidad ni `.env`.

Un sitio no tiene proceso propio: desplegarlo es construir y **publicar**. El build nuevo
se copia junto al viejo, toma su dueño y sus permisos, y entra por dos `mv` — el sitio
sirve el build viejo entero o el nuevo entero, nunca una copia a medias. Un build sin
`index.html` se rechaza antes de tocar nada. No se reinicia nada: nginx lee del disco en
cada petición. Por eso a un sitio no se le ofrece restart, stop ni logs, y no tiene
comprobación de disponibilidad: su comprobación es la propia publicación.

### Saber cuándo termina un despliegue

Nada informa de que una aplicación esté lista. systemd da un unit por arrancado en cuanto
el proceso se bifurca, mucho antes de que algo escuche. Así que es una pregunta repetida
hasta que se responde o se agota el presupuesto — que es lo que hace por debajo todo lo
que afirme otra cosa.

Se pregunta **desde el host**, no desde dentro del contenedor, porque es desde donde
preguntará nginx. Un servicio atado a `127.0.0.1` pasa una comprobación interna y no sirve
a nadie. Y el bucle abandona en cuanto el unit muere, para que un arranque fallido se vea
en segundos y no al cabo del presupuesto entero.

Los pasos de instalación y construcción **tienen que terminar**: corren bajo `timeout`
dentro del contenedor, así que un comando que no vuelve muere ahí en vez de quedar
huérfano. El que sigue corriendo es el de arranque, y de ese se ocupa systemd.

### Snapshots

Dos orígenes que no pueden confundirse: los que tomó una persona, y los que tomamos
nosotros antes de cada despliegue. La regla que los separa es la misma de los vhosts —
el prefijo dice quién lo hizo.

```
croft-deploy-<servicio>-<fecha>    nuestro, podable
before-upgrade                     de quien sea, intocable
```

Se conservan tres por servicio, y nunca el marcado como sano. Cada borrado es un paso
visible del plan: este es el único sitio donde croft elimina algo por su cuenta.

**Dos formas de volver atrás**, que deshacen cosas distintas:

| | Alcance | Recupera datos |
|---|---|---|
| Fijar el commit anterior | solo ese servicio | no |
| Restaurar el snapshot | el contenedor entero | sí |

La primera es quirúrgica y sirve para el fallo normal, que es código malo. La segunda es
lo único que salva de una migración que destrozó la base de datos — y es lo que una imagen
de Docker no puede hacer. Por eso el checkout baja con diez commits de historia y no uno.

### La consecuencia que hay que decir en voz alta

Los snapshots son **del contenedor**, no del servicio. Restaurar uno se lleva a sus
vecinos por delante. Por eso un servicio por contenedor suele ser más sensato, y por eso
el panel lo avisa **al añadir el segundo** —que es cuando empieza a costar algo— y otra
vez en el plan de rollback, nombrando qué más se revierte.

### Lo que falta por hacer aquí

- Los snapshots de un nombre de servicio abandonado no se podan nunca: la poda solo
  alcanza al servicio que se está desplegando.
- Un sitio estático *desplegado desde cero* se sirve todavía con un truco (`serve`) porque
  croft no escribe configuración de nginx dentro del contenedor. Uno *adoptado* ya se
  publica en el nginx que tenía; el paso natural es ofrecer lo mismo al desplegar, cuando
  el contenedor ya tiene un nginx.
- La adopción es solo desde el panel: no hay comando de CLI todavía.
- Repos privados: `AuthKind` y `Source.Private()` existen, las claves de despliegue no.

## Camino a PaaS

Lo que queda se añade como módulos nuevos siguiendo las mismas capas:

| Módulo | Domain services | Repositorios de infraestructura |
|---|---|---|
| `database` | `ProvisionDatabase`, `DestroyDatabase` | el mismo contenedor + `lxc snapshot` |
| `secret` | `Generate`, `Rotate` | el fichero de entorno del servicio |

Ninguno obliga a tocar `instance`, `route` ni `certificate`. Esa es la prueba de que la
separación en capas está bien puesta: las features nuevas se añaden por los extremos, no
por el centro.

`lxc snapshot` da backups y rollback prácticamente gratis, lo que es una ventaja real
frente a los PaaS basados en Docker, donde el estado persistente es siempre el problema
difícil.

## Lo que OpenCroft no va a hacer

Delimitar esto ahora evita que el producto se disuelva:

- **No orquesta clústeres.** Un servidor, muchos contenedores. Si necesitas Kubernetes,
  usa Kubernetes.
- **No gestiona virtualización.** Eso es Proxmox y lo hace mejor.
- **No esconde el servidor.** Si una operación no se puede explicar como comandos que el
  usuario podría haber escrito, no entra.
