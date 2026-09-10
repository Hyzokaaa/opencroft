# Roadmap

Plan de construcción de OpenCroft. Cada fase es entregable y usable por sí sola — no hay
fases que solo tengan sentido si se completa la siguiente.

La arquitectura de referencia está en [ARCHITECTURE.md](./ARCHITECTURE.md).

---

## Fase 0 — Sanear lo que hay

Los scripts actuales son un prototipo válido de las primitivas, pero tienen fallos que no
deben heredarse. Antes de escribir código nuevo, dejar la base honesta.

- [ ] Sacar `pr.sh` a otro repositorio — no tiene relación con infraestructura
- [ ] **IP fija en nginx**: `deploy-instance.sh:146` congela una IP de DHCP. Si el
      contenedor reinicia y cambia de IP, el proxy queda roto y nada lo detecta.
      Pasar a direcciones estáticas asignadas por la herramienta
- [ ] **Puerto interno fijo**: se asume siempre el 80 dentro del contenedor. Hacerlo
      configurable
- [ ] **Fuga de variables**: `destroy-instance.sh:32-35` hace `source` dentro de un bucle;
      si un `.conf` no define `DOMAIN`, hereda el de la iteración anterior y muestra datos
      erróneos
- [ ] **Ejecución de código**: `destroy-instance.sh:51` hace `source` sobre un fichero de
      `/etc` corriendo como root. Cualquier cosa escrita ahí se ejecuta. Parsear, no
      ejecutar
- [ ] Añadir `list-instances.sh` — es el comando más usado en una herramienta así y falta
- [ ] Mover los metadatos de `/etc/lxc-instances/*.conf` a anotaciones `user.croft.*` en
      LXD. Los `.conf` pasan a ser caché, no verdad

**Entregable**: los scripts actuales, sin los bugs, con inventario.
**Valor**: utilizable ya mismo, y valida el modelo de anotaciones antes de construir sobre él.

---

## Fase 1 — Núcleo y CLI

El binario de Go con la arquitectura por capas y las primitivas completas. Sin daemon,
sin UI todavía.

**Módulos**: `instance`, `route`, `certificate`, `host`, `shared/id`, `shared/host`

- [ ] Esqueleto del proyecto y abstracción `Host` con implementación `LocalHost`
- [ ] Driver `ContainerRuntime` con implementaciones para LXD e Incus, detectadas al arrancar
- [ ] Driver `Proxy` sobre `/etc/nginx/croft.d/`, sin asumir el layout de Debian
- [ ] Módulo `instance`: crear, listar, ver, arrancar, parar, destruir, editar límites
- [ ] Módulo `route`: añadir dominio, listar, quitar, cambiar destino y puerto
- [ ] Módulo `certificate`: emitir, renovar, revocar, listar con fecha de caducidad
- [ ] ACME embebido con `lego`, sin dependencia de certbot
- [ ] Proveedores DNS para el reto ACME: OVH y Cloudflare
- [ ] Asignación de IP estática y validación de que las rutas apuntan a la IP real
- [ ] Cabecera `managed-by: croft` con hash en todos los ficheros generados
- [ ] Modo `--plan` y `--explain` en toda operación de escritura
- [ ] Descubrimiento de recursos `unmanaged` y comando de adopción
- [ ] Salida `--json` en todos los comandos de lectura
- [ ] Suite de tests con mocks y `FakeHost`

**Criterio de aceptación de la fase**: todo comando funciona de forma no interactiva con
flags. El modo interactivo, si existe, es una envoltura por encima. Sin esto la UI tendría
que reimplementar la lógica, que es precisamente lo que no queremos.

**Entregable**: `croft` instalable con un binario.
**Valor**: ya sustituye a los scripts con ventaja clara.

---

## Fase 2 — Daemon, API y UI de lectura

El 80% del valor percibido con el 0% del riesgo. Solo lectura.

- [x] `croft serve` — mismo binario, modo daemon
- [x] API HTTP con `hostId` en las rutas desde el principio
- [x] Autenticación: usuarios locales y sesiones
- [ ] Tokens de API para automatización
- [ ] SQLite para estado propio (usuarios, sesiones, tokens, auditoría, trabajos)
- [x] Separación en dos procesos: `croft agent` (root) y `croft serve` (sin privilegios), por unix socket
- [x] UI en React + Vite + Tailwind, embebida con `go:embed`
- [x] Dashboard: instancias, estado, rutas y hallazgos
- [x] **Avisos**: certificados que caducan pronto, rutas que apuntan a instancias caídas o
      inexistentes, instancias sin dominio, drift detectado
- [x] Los recursos externos se listan y se distinguen
- [ ] Adoptarlos explícitamente
- [ ] Log de auditoría consultable

**Entregable**: un panel que te dice qué hay corriendo en tu servidor y qué está mal.
**Valor**: esto solo ya justifica el producto. Un vhost apuntando a un contenedor muerto o
un certificado que caduca en 9 días es exactamente lo que nadie detecta a tiempo.

---

## Fase 3 — Escritura desde la UI

- [x] Crear y destruir instancias desde el panel
- [ ] Editar límites de CPU y memoria
- [ ] Gestión de dominios y certificados
- [x] Cola de trabajos con logs en streaming por SSE
- [x] Pantalla de confirmación con el plan: los comandos exactos que se van a ejecutar
- [x] Modo global de comandos en la barra superior
- [x] Detección de drift en la UI

**Regla de la fase**: cada acción de la UI llama al mismo Command que el CLI. Si aparece
lógica de negocio en un handler HTTP, está mal puesta.

**Entregable**: panel de control completo para LXD/Incus + nginx + TLS.
**Valor**: producto v1 vendible. Aquí se puede cortar y tener algo coherente.

---

## Fase 4 — Operación diaria

Lo que convierte un panel en una herramienta que se usa todos los días.

- [ ] Consola web por websocket contra `lxc exec`
- [ ] Visor de logs del contenedor
- [ ] Snapshots: crear, listar, restaurar, programar (`lxc snapshot`)
- [ ] Métricas históricas de CPU, memoria y disco
- [ ] Explorador de ficheros del contenedor
- [ ] Renovación automática de certificados
- [ ] Notificaciones por correo o webhook ante avisos

---

## Fase 5 — Equipos

Hoy un usuario es solo alguien que puede entrar. Cuando OpenCroft gestione servidores de
terceros —el caso de un MSP— hace falta saber quién puede tocar qué.

- [ ] Correo en el usuario, además del nombre
- [ ] Organizaciones, y usuarios pertenecientes a ellas
- [ ] Proyectos dentro de una organización, agrupando instancias y dominios
- [ ] Roles por organización y por proyecto
- [ ] Invitaciones por correo
- [ ] Log de auditoría con el actor de cada operación

**Cuidado con la fuente de verdad.** Las organizaciones y los roles son datos propios y
viven en SQLite: no se pueden derivar del sistema. Pero la pertenencia de una instancia a
un proyecto sí es una anotación del contenedor, como el resto de su estado deseado. Si
acaba en una tabla, se pierde al migrar el contenedor y se rompe el principio.

**El correo trae una dependencia nueva**: enviar invitaciones necesita SMTP. Eso convierte
el binario sin dependencias en algo que requiere configuración de correo. Conviene que sea
opcional y que exista siempre la vía de crear la cuenta desde el host.

## Fase 6 — Catálogo de aplicaciones

Antes que el PaaS genérico, porque da valor inmediato con mucho menos trabajo y conecta
con el resto del ecosistema.

- [ ] Formato de receta de instalación (contenedor + dominio + certificado + servicio)
- [ ] Catálogo con las aplicaciones autoalojadas habituales
- [ ] `croft app install open-helpdesk --domain soporte.ejemplo.com`
- [ ] Actualización y desinstalación limpias

Las recetas son ficheros legibles y editables: instalar desde el catálogo y luego tocar la
instancia a mano tiene que seguir funcionando.

---

## Fase 7 — PaaS

Módulos nuevos siguiendo las mismas capas. No requiere tocar los existentes.

- [ ] Módulo `app`: origen git, despliegue, historial, rollback
- [ ] Módulo `build`: detección de runtime, buildpacks o Dockerfile
- [ ] Módulo `env`: variables de entorno y secretos cifrados
- [ ] Webhooks de despliegue (GitHub, GitLab)
- [ ] Módulo `database`: PostgreSQL, MySQL y Redis gestionados con backups
- [ ] Promoción de una instancia existente a App

---

## Fase 8 — Multi-host

Solo cuando haya usuarios pidiéndolo. La preparación ya está hecha desde la fase 1.

- [ ] Implementación `RemoteHost` sobre el protocolo nativo de LXD
- [ ] Agente ligero para las operaciones de nginx y TLS
- [ ] Selector de servidor en la UI
- [ ] Permisos por servidor

---

## Fuera de alcance

- Orquestación de clústeres — para eso está Kubernetes
- Gestión de virtualización — para eso está Proxmox
- Cualquier operación que no se pueda explicar como comandos que el usuario podría haber
  escrito a mano

---

## Riesgos

**Competencia.** Coolify y Dokploy son gratuitos y open source. Proxmox ya gestiona LXC
con UI y es el estándar de facto. La diferenciación real no es la lista de features, es el
enfoque: contenedores de sistema completos, transparencia total, y un servidor que sigue
siendo del usuario.

**El principio de "todo editable a mano" es fácil de escribir y difícil de mantener.**
Cada feature nueva empuja hacia guardar estado propio en SQLite porque es más cómodo.
Tratarlo como restricción dura, no como aspiración: si un repositorio de infraestructura
necesita SQLite, la feature está mal diseñada.

