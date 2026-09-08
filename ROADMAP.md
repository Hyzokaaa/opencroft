# Roadmap

Plan de construcción de OpenCroft. Cada fase es entregable y usable por sí sola — no hay
fases que solo tengan sentido si se completa la siguiente.

La arquitectura de referencia está en [ARCHITECTURE.md](./ARCHITECTURE.md).
El posicionamiento y el modelo de sostenibilidad, en [POSITIONING.md](./POSITIONING.md) y
[MONETIZATION.md](./MONETIZATION.md).

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

- [ ] `croft serve` — mismo binario, modo daemon
- [ ] API HTTP con `hostId` en las rutas desde el principio
- [ ] Autenticación: usuarios locales, sesiones, tokens de API
- [ ] SQLite para estado propio (usuarios, sesiones, tokens, auditoría, trabajos)
- [ ] Separación `croft-api` (sin privilegios) / `croft-agent` (root) por unix socket
- [ ] UI en React + Vite + Tailwind, embebida con `go:embed`
- [ ] Dashboard: instancias, estado, CPU/RAM en uso, rutas, certificados
- [ ] **Avisos**: certificados que caducan pronto, rutas que apuntan a instancias caídas o
      inexistentes, instancias sin dominio, drift detectado
- [ ] Vista de recursos `unmanaged` con opción de adoptar
- [ ] Log de auditoría consultable

**Entregable**: un panel que te dice qué hay corriendo en tu servidor y qué está mal.
**Valor**: esto solo ya justifica el producto. Un vhost apuntando a un contenedor muerto o
un certificado que caduca en 9 días es exactamente lo que nadie detecta a tiempo.

---

## Fase 3 — Escritura desde la UI

- [ ] Crear y destruir instancias desde el panel
- [ ] Editar límites de CPU y memoria
- [ ] Gestión de dominios y certificados
- [ ] Cola de trabajos con logs en streaming por SSE
- [ ] Pantalla de confirmación con el plan: comandos exactos y diff de ficheros
- [ ] Panel "equivalente CLI" en cada operación
- [ ] Detección de drift en la UI con diff visible y decisión explícita del usuario

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
- [ ] Renovación automática de certificados con notificación
- [ ] Notificaciones por correo o webhook ante avisos

---

## Fase 5 — Catálogo de aplicaciones

Antes que el PaaS genérico, porque da valor inmediato con mucho menos trabajo y conecta
con el resto del ecosistema.

- [ ] Formato de receta de instalación (contenedor + dominio + certificado + servicio)
- [ ] Catálogo con las aplicaciones autoalojadas habituales
- [ ] `croft app install open-helpdesk --domain soporte.ejemplo.com`
- [ ] Actualización y desinstalación limpias

Las recetas son ficheros legibles y editables: instalar desde el catálogo y luego tocar la
instancia a mano tiene que seguir funcionando.

---

## Fase 6 — PaaS

Módulos nuevos siguiendo las mismas capas. No requiere tocar los existentes.

- [ ] Módulo `app`: origen git, despliegue, historial, rollback
- [ ] Módulo `build`: detección de runtime, buildpacks o Dockerfile
- [ ] Módulo `env`: variables de entorno y secretos cifrados
- [ ] Webhooks de despliegue (GitHub, GitLab)
- [ ] Módulo `database`: PostgreSQL, MySQL y Redis gestionados con backups
- [ ] Promoción de una instancia existente a App

---

## Fase 7 — Multi-host

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

**Foco.** Este es el riesgo mayor y no es técnico. OpenCroft compite con proyectos maduros,
gratuitos y con comunidad. Como complemento que reduce la fricción de autoalojar
open-helpdesk, es una idea muy buena. Como segundo producto que compite por la misma
atención, puede hundir a los dos. Ver [POSITIONING.md](./POSITIONING.md).

**Perseguir la paridad con Coolify.** El 90% de su superficie son features que casi nadie
usa. Cortar en la fase 3 y consolidar eso es mejor estrategia que llegar a la fase 6 sin
usuarios.
