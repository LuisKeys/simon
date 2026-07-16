# Simon SDK (simon-go) — Documento maestro para presentación

> **Propósito de este documento.** Es la fuente única para armar una
> presentación de PowerPoint (por ejemplo, con Claude como plugin de
> PowerPoint) sobre las funcionalidades del SDK **Simon** en su versión Go
> (`simon-go`). Está organizado en **secciones = grupos de diapositivas**.
> Cada sección incluye: un título sugerido para la slide, los puntos clave
> (bullets listos para pegar), y notas del orador donde hace falta contexto.
> No contiene código: los ejemplos se describen por **nombre y para qué
> sirven**, no por su implementación.

---

## Cómo usar este documento para armar el PowerPoint

- Cada encabezado `##` corresponde a **una diapositiva o un pequeño grupo de
  diapositivas**.
- Los bullets bajo cada sección están redactados para copiarse casi tal cual
  al cuerpo de la slide.
- Los bloques **"Nota del orador"** dan el contexto para explicar de viva voz;
  no van en la slide.
- Sugerencia de duración: presentación de ~20–30 minutos, ~18–22 slides.
- Orden narrativo recomendado: **Qué es → Por qué existe → Arquitectura →
  Los 4 pilares funcionales → Ejemplos ejecutables → Configuración → Cierre.**

---

## 1. Portada

**Título de slide:** Simon SDK — Framework ligero de agentes de IA (port a Go)

- Framework liviano para construir **agentes de IA** en Go.
- Port fiel de "Simon SDK", originalmente escrito en Python.
- Filosofía: **mínimas dependencias, local-first, privacidad por diseño**.
- Subtítulo sugerido: *"Del prototipo en Python a un binario Go sin CGO,
  multiplataforma y compilable de forma cruzada."*

> **Nota del orador:** Simon nació en Python como framework educativo y
> ligero. Esta versión Go conserva el comportamiento y los límites de
> paquetes del original, pero reemplaza los "modismos" de Python que no
> tienen equivalente directo en Go. Ese es el hilo conductor de toda la
> charla.

---

## 2. ¿Qué es Simon? (elevator pitch)

**Título de slide:** ¿Qué resuelve Simon?

- Un **agente** que razona y actúa en bucle (patrón **ReAct**): piensa,
  llama herramientas, observa el resultado y repite hasta responder.
- **Multi-proveedor**: OpenAI, Anthropic y Ollama (local) detrás de una única
  interfaz; selección automática de modelo según la tarea.
- **RAG incorporado**: base de conocimiento con extracción de documentos,
  embeddings e índice vectorial propio.
- **Pipeline de actividad local-first**: sensores → eventos → clasificación
  semántica → sesiones → descubrimiento de hábitos, con **privacidad
  deny-by-default**.
- **Superficie de uso lista**: CLI, TUI de chat, planificador de objetivos y
  cliente MCP.

> **Nota del orador:** Cuatro grandes bloques funcionales. El resto de la
> charla los recorre uno por uno.

---

## 3. Principios de diseño (por qué se ve como se ve)

**Título de slide:** Decisiones de diseño clave

- **Sin dualidad async.** Python ofrece `run()`/`run_async()`; Go usa un único
  método síncrono y logra concurrencia con **goroutines**.
- **Sin esquemas de herramientas por reflexión de firma.** En Go los binarios
  no conservan nombres de parámetros: las herramientas se declaran con un
  **struct de parámetros explícito** (tags `json`/`jsonschema`).
- **Identidad de errores por multi-unwrap** (Go 1.20+): un mismo error puede
  matchear tanto un sentinel de dominio como uno de convención de stdlib.
- **Formatos de disco propios** (no hay compatibilidad binaria con Python).
- **Frontera de privacidad local-first**: la clasificación semántica siempre
  usa Ollama local, sin importar qué claves cloud estén configuradas.
- **Los sensores no son herramientas**: son bucles de sondeo en segundo plano,
  no funciones que el LLM decide invocar.
- **Sin CGO / compilable de forma cruzada** (por eso los sensores de macOS
  quedan fuera de alcance en este port).

> **Nota del orador:** Estos principios explican casi todas las diferencias
> con el original en Python. Vale la pena mencionarlos temprano porque
> reaparecen en cada capa.

---

## 4. Arquitectura en cuatro capas

**Título de slide:** Arquitectura — 4 capas mayormente independientes

- **Superficie:** `cmd/simon` (CLI), `tui`, `planner`, `mcp`.
- **Núcleo de agente:** `agent → model → tool`, más `multi`, `memory`,
  `router`, `reliability`.
- **Base de conocimiento (RAG):** `knowledge` (+ `embed`, `index`, `extract`).
- **Pipeline de actividad:** `sensors → events → privacy → semantic →
  activity → habits`.

**Puntos de acoplamiento (para el diagrama):**
- La superficie depende del núcleo (y de conocimiento para `index`/`plan`).
- El núcleo **no importa** conocimiento directamente: se conecta vía la
  interfaz `KnowledgeSearcher` (evita arrastrar dependencias de embeddings a
  cualquier binario que solo quiera un agente básico).
- El pipeline de actividad está **totalmente separado** del núcleo de agente.
- Solo `pkg/simonerr` es público; todo lo demás vive en `internal/`.

> **Nota del orador (sugerencia visual):** Un diagrama de 4 cajas apiladas es
> ideal para esta slide. La independencia entre capas es un punto de venta:
> se puede usar solo el agente, solo el RAG, o solo el pipeline.

---

# PILAR 1 — Núcleo de agente

## 5. El bucle ReAct: `Agent.Run`

**Título de slide:** El corazón: el bucle ReAct

- Un `Agent` se construye con opciones: memoria, herramientas, system prompt,
  modelo, máximo de pasos (por defecto **6**), hooks de eventos, conocimiento.
- `Run(ctx, prompt)` ejecuta el ciclo:
  1. **Resuelve el modelo** (router o override) y emite evento `model_selected`.
  2. **Siembra los mensajes** (system prompt + memoria + prompt del usuario).
  3. **Atajo de herramienta** opcional: un prompt con formato `tool:nombre
     {json}` llama la herramienta directo, sin pasar por el LLM.
  4. **Inyecta contexto de conocimiento** si hay una base adjunta (top-2).
  5. **Primera respuesta del modelo** (envuelta en reintentos).
  6. **Bucle de herramientas:** mientras el modelo pida llamadas y no se
     alcance `MaxSteps`, ejecuta cada herramienta y realimenta el resultado.
  7. **Persiste y emite** `response_received` con uso de tokens y nº de pasos.

> **Nota del orador:** Este es el diagrama más importante de la charla. Un
> flujo vertical de 7 pasos con la flecha de "loop" entre el paso 6 y el
> modelo comunica todo el valor del SDK.

---

## 6. Salida estructurada y observabilidad

**Título de slide:** Salida tipada + eventos observables

- **`RunStructured[T]`** (genéricos de Go): fuerza la salida del modelo a un
  tipo `T` (el equivalente Go de un modelo Pydantic). Tolera "ruido" de
  formato (bloques ```` ``` ````), reintenta ante fallos de parseo y devuelve
  un error recuperable con el texto crudo si se agotan los intentos.
- **Cuatro eventos** para observabilidad, en puntos fijos del ciclo:
  `model_selected`, `tool_called`, `retry_attempted`, `response_received`.
- Se consumen registrando un handler (`WithOnEvent`) — útil para logging,
  métricas y cálculo de costos/tokens.

> **Nota del orador:** Buen momento para vincular con el ejemplo
> `structured_output_agent` y `hooks_agent` (los verán más adelante).

---

## 7. Proveedores de modelo y router

**Título de slide:** Multi-proveedor con selección automática

- Interfaz única `Model.Complete(...)`; los SDKs de terceros viven en
  subpaquetes aislados:
  - **OpenAI** — soporta llamadas a herramientas.
  - **Anthropic** — soporta llamadas a herramientas.
  - **Ollama** (local) — **no** soporta herramientas (se ignora ese
    parámetro); no streaming.
  - **EchoModel** — fallback sin red (responde "eco"), ideal para tests
    deterministas y para correr ejemplos sin credenciales.
- **Router** (`Resolve`) elige proveedor + modelo:
  - Una etiqueta explícita de proveedor **gana** si está configurada.
  - Si no, decide por **complejidad de la tarea** (heurística de palabras
    clave, con equivalentes en español):
    - Tarea **compleja** → OpenAI, luego Anthropic, luego Ollama (cloud-first).
    - Tarea **simple** → Ollama, luego OpenAI, luego Anthropic (local-first).
  - Si no hay nada configurado → EchoModel.

> **Nota del orador:** El router NO construye el modelo (evita un ciclo de
> importación); solo devuelve la *elección*. Detalle fino que muestra el
> cuidado arquitectónico, pero puede omitirse si el público no es técnico.

---

## 8. Herramientas (tools)

**Título de slide:** Herramientas: cómo el agente actúa sobre el mundo

- `New[P]` construye una herramienta reflejando un **struct de parámetros**
  hacia un JSON Schema (reemplaza al decorador `@tool` de Python).
- `NewRaw` omite la reflexión: para herramientas cuyo esquema solo se conoce
  en runtime (así consume Simon las herramientas **MCP**).
- `Registry` + `RunToolCall`: **despacho único** compartido por el agente y el
  runner. Una herramienta faltante, argumentos mal formados o un error de la
  función se devuelven **como resultado de error al modelo**, sin tumbar la
  ejecución.
- **`tool.Runner`**: el mismo bucle modelo↔herramienta pero **turno por
  turno**, para inspeccionar o intervenir entre turnos (iterador
  `range`-over-func). `UntilDone` lo lleva hasta la respuesta final.

> **Nota del orador:** El mensaje clave: los errores de herramienta son
> "datos" para el modelo, no crashes. Esto hace agentes robustos.

---

## 9. Memoria y confiabilidad

**Título de slide:** Memoria conversacional + reintentos

- **Memoria** (interfaz `Add`/`List`/`Clear`), dos implementaciones:
  - **InMemory** — slice protegido con mutex, dura lo que vive el proceso.
  - **JSONFile** — un archivo JSON legible por conversación (bajo
    `.simon_chats/`); **sobrevive reinicios del proceso**. Sin path-traversal
    (usa solo el nombre base).
- **Reliability** — helper genérico `Retry` con **backoff exponencial** y
  timeout por intento; parámetros configurables por variables de entorno.

> **Nota del orador:** Diferencia importante frente a Python: el mutex existe
> porque en Go varias goroutines pueden tocar la memoria en paralelo.

---

## 10. Patrones multi-agente

**Título de slide:** Varios agentes trabajando juntos

- **Group** — corre **N agentes idénticos** en paralelo sobre el **mismo**
  prompt (analista / crítico / resumidor).
- **Pool** — corre **agentes heterogéneos** (cada uno su prompt) en paralelo,
  preservando el orden de entrada; opción de reportar errores por tarea en vez
  de fallar todo.
- **Triage** — un agente router interno **elige el especialista** más adecuado
  (código / matemática / redacción) con una sola llamada al LLM.
- Todo con **goroutines + WaitGroup**, no con `asyncio.gather`.

> **Nota del orador:** Vincular con los ejemplos `parallel_agents`,
> `agent_pool_example` y `triage_agent`.

---

## 11. Manejo de errores (`pkg/simonerr`)

**Título de slide:** Errores con doble identidad

- Único paquete público del SDK.
- Reemplaza la herencia múltiple de excepciones de Python con **sentinels +
  multi-unwrap**: `errors.Is` puede matchear la identidad de **dominio**
  (p. ej. "error de proveedor") o de **convención stdlib** (p. ej. "runtime")
  sobre el mismo valor.
- Tipos cubiertos: proveedor, herramienta, conocimiento, permiso denegado, y
  un error específico de **salida estructurada** (recupera el texto crudo y el
  nº de intentos).

> **Nota del orador:** Slide opcional / técnica. Si el público es de negocio,
> se puede fusionar con la de confiabilidad o saltar.

---

# PILAR 2 — Base de conocimiento (RAG)

## 12. RAG: de documentos a respuestas

**Título de slide:** Base de conocimiento (RAG) end-to-end

- **`Add`**: extrae texto de un archivo o carpeta, lo **chunkea** (ventanas de
  caracteres solapadas), lo **embebe** y lo persiste. Salta lo ya indexado
  salvo `force`.
- **`Search`**: embebe la consulta y devuelve los fragmentos más relevantes
  (con fuente y score).
- **Extracción de documentos** soportada: **PDF, DOCX, XLSX, PPTX** y texto
  plano (fallback tolerante a bytes inválidos).
- **Proveedores de embeddings**: OpenAI (por defecto,
  `text-embedding-3-small`), Ollama (local) y Voyage (recomendado por
  Anthropic). Todos los vectores se **normalizan L2**.

> **Nota del orador:** El chunking es por **caracteres/runas**, no por tokens
> —fiel al original—; simple y predecible.

---

## 13. El índice vectorial propio (SIDX)

**Título de slide:** SIDX — índice vectorial desde cero

- Formato binario propio **"SIDX"** que reemplaza el pickle+numpy de Python
  (sin compatibilidad binaria con el `.simon_knowledge/` de Python).
- Por fuente en disco: un `.sidx` (cabecera + vectores `float32`), un
  `.meta.json` (textos + fuente) y un `manifest.json` compartido.
- **Búsqueda por fuerza bruta** (producto punto = similitud coseno, porque los
  vectores ya vienen normalizados); apropiado para la escala del SDK.
- La interfaz `Index` deja la puerta abierta a un backend ANN futuro sin
  cambiar los llamadores.
- **Guarda de seguridad**: detecta mezcla de dimensiones/modelos de embedding
  y falla explícitamente (no se pueden mezclar espacios de embeddings).

> **Nota del orador:** Vincular con el ejemplo `knowledge_agent`, que indexa
> el paper "Attention Is All You Need" y responde preguntas solo desde él.

---

# PILAR 3 — Pipeline de actividad (local-first)

## 14. El flujo de datos de actividad

**Título de slide:** Pipeline de actividad: de la señal al hábito

- Flujo: **Sensor → EventBus → clasificación semántica → compresión en
  sesiones → almacén de actividad + grafo de transiciones → minería de
  hábitos.**
- **EventBus** pub/sub: persiste cada evento (SQLite puro Go, sin CGO) y lo
  reparte a los suscriptores **en paralelo**; los errores de handler se
  loguean, nunca tumban al publicador.
- **EventCompressor**: una corrida ininterrumpida de la misma categoría es
  **una sola sesión**, por larga que sea.

> **Nota del orador:** "Local-first" es el titular. Todo esto corre en el
> dispositivo; nada se manda a la nube por defecto.

---

## 15. Privacidad y clasificación semántica

**Título de slide:** Privacidad por diseño

- **PermissionManager deny-by-default**: nada se observa sin permiso explícito.
  Scopes finos: `window_focus`, `clipboard_metadata`, `clipboard_content`,
  `screen_text` — se puede permitir "hubo actividad de portapapeles" **sin**
  permitir ver qué se copió.
- **Auditoría**: cada grant/revoke/deny se publica como evento en el mismo bus
  → "qué observó el sistema y por qué" es respondible desde el log.
- **Clasificación semántica** (`semantic`): usa **siempre Ollama local**,
  saltándose el router a propósito — títulos de ventana y portapapeles
  **nunca salen del dispositivo**, sin importar las claves cloud configuradas.
  Solo señales de texto; nunca capturas de pantalla. Categorías por defecto:
  programación, terminal, lectura de docs, chat, email, navegación, etc.

> **Nota del orador:** Este es el mensaje diferencial más fuerte para
> audiencias sensibles a privacidad. Enfatizar el "bypass deliberado del
> router" como decisión de seguridad, no un descuido.

---

## 16. Actividad y descubrimiento de hábitos

**Título de slide:** De sesiones a hábitos

- **Modelos de lectura** sobre el flujo de sesiones:
  - **ContextEngine** — "qué está pasando ahora" en memoria, con historial
    reciente acotado.
  - **GraphBuilder / GraphStore** — grafo de **transiciones** entre categorías
    (de→a, con conteo), en SQLite.
- **HabitDiscoveryEngine** — **minería de n-gramas** sobre el historial de
  sesiones (análisis por lotes, no por evento):
  - Detecta patrones que ocurren en al menos N días distintos, **a
    aproximadamente la misma hora del día**.
  - Calcula confianza y evita re-publicar hallazgos que no cambiaron
    materialmente.
- **Sensores**: existe la **interfaz** y el ciclo de vida (Manager), pero **no
  se incluye implementación concreta** en este port. Los sensores de macOS
  (PyObjC en Python) quedan fuera de alcance para no romper el build sin CGO;
  el diseño previsto es un **proceso satélite en Swift** que emite JSON por
  línea.

> **Nota del orador:** Aclarar que este pilar está "cableado y testeado
> punta a punta" pero que la captura real de sensores en macOS es trabajo
> futuro documentado, no una promesa vacía.

---

# PILAR 4 — Superficie de uso

## 17. CLI, TUI, Planner y MCP

**Título de slide:** Cómo se usa Simon en la práctica

- **CLI `simon`** (solo stdlib `flag`), cuatro subcomandos:
  - `chat` — chat interactivo con memoria en TUI.
  - `ask "<prompt>"` — una sola respuesta y salida.
  - `index <ruta>` — indexa documentos en la base de conocimiento.
  - `plan "<objetivo>"` — planifica y ejecuta un objetivo.
- **TUI de chat** — bucle de chat con **render de Markdown → ANSI** (headers,
  bullets, negrita, código, bloques). Comandos `/quit` y `/clear`. (La única
  función deliberadamente omitida vs. Python: el autocompletado por tabulación.)
- **Planner** — descompone un objetivo en tareas con **una llamada al LLM** y
  las ejecuta **secuencialmente**, mostrando un checklist en vivo; aborta ante
  el primer error devolviendo el avance parcial.
- **Cliente MCP** — conecta a un servidor **MCP** por stdio y expone sus
  herramientas como herramientas Simon: para el agente son
  **indistinguibles** de las locales.

> **Nota del orador:** Dato interesante para MCP: cada llamada abre una
> conexión stdio fresca (stateless), fiel al original en Python.

---

## 18. Ejemplos ejecutables — visión general

**Título de slide:** ~15 ejemplos, uno por funcionalidad

- El repo trae ~15 programas pequeños en `examples/`, **cada uno demuestra una
  funcionalidad**.
- Se corren con `go run ./examples/<nombre>` **desde la raíz del repo**.
- La mayoría necesita credenciales de un proveedor (o un Ollama local); **dos
  no necesitan nada**: `tool_runner_example` y `activity_pipeline_example`.

> **Nota del orador:** Las próximas dos slides son la "galería" de ejemplos.
> Se pueden partir en varias slides si se prefiere una tabla por pilar.

---

## 19. Ejemplos — Núcleo de agente

**Título de slide:** Ejemplos: agentes, herramientas y multi-agente

- **`basic_agent`** — el agente mínimo: solo defaults y una única llamada
  `Run`. Punto de partida.
- **`builtin_tools_agent`** — herramientas declaradas a mano (fecha/hora,
  listar/leer/escribir archivos, ejecutar shell, búsqueda web stub) invocadas
  con el atajo `tool:nombre {json}`. Muestra por qué en Go las herramientas son
  structs explícitos.
- **`hooks_agent`** — observabilidad: maneja los cuatro tipos de evento e
  imprime el uso total de tokens al final.
- **`memory_agent`** — recuerdo conversacional **en memoria** entre dos
  llamadas `Run` seguidas.
- **`persistent_memory_agent`** — memoria **persistente entre procesos**
  (archivo JSON). Correrlo dos veces demuestra que el recuerdo sobrevive al
  reinicio.
- **`structured_output_agent`** — salida **tipada** (`RunStructured[Recipe]`),
  el análogo Go de un modelo Pydantic.
- **`run_context_example`** — reemplazo idiomático de los `contextvars` de
  Python: estado por request vía **closures + una goroutine/agente por
  request**, mostrado en secuencial y concurrente.
- **`tool_runner_example`** — el bucle **turno por turno** con intervención
  manual entre turnos. Usa `EchoModel`: **no necesita API key ni red**.
- **`parallel_agents`** — agentes **homogéneos** en paralelo (Group) sobre el
  mismo prompt.
- **`agent_pool_example`** — agentes **heterogéneos** en paralelo (Pool);
  wall-clock = el agente más lento.
- **`triage_agent`** — **enrutamiento** de tareas a especialistas (Triage)
  mediante un agente router interno.

---

## 20. Ejemplos — Conocimiento, superficie y actividad

**Título de slide:** Ejemplos: RAG, planner, MCP, TUI y pipeline

- **`knowledge_agent`** — **RAG completo**: indexa un PDF (el paper
  "Attention") y responde 5 preguntas contestables solo desde ese documento.
- **`chat_tui`** — chat interactivo en TUI con un agente con **personalidad**
  ("Luke", un chef experto) y memoria activada.
- **`planner_agent`** — **descomposición de objetivo + ejecución secuencial**,
  imprimiendo un checklist vivo en cada cambio de estado.
- **`mcp_agent`** — consume herramientas de un **servidor MCP externo** dentro
  de un agente; encadena dos llamadas (`add_numbers` y luego `reverse_string`).
- **`mcp_agent/server`** — el **servidor MCP stdio** de acompañamiento que
  expone esas dos herramientas; no se corre solo.
- **`activity_pipeline_example`** — el **pipeline de actividad completo**
  (privacidad → bus → semántico → sesiones → grafo/contexto → hábitos),
  sembrado con 5 días de historia sintética para disparar la detección de
  patrones. **Degrada con gracia** si no hay Ollama local; no requiere
  credenciales.

> **Nota del orador:** `knowledge_agent` y `activity_pipeline_example` son los
> mejores para una demo en vivo: el primero muestra RAG "de verdad", el
> segundo corre sin credenciales.

---

## 21. Configuración

**Título de slide:** Configuración por entorno (`.env`)

- Se configura con un archivo **`.env`** en el directorio de trabajo (no pisa
  variables ya presentes en el entorno real). Copiar `.env.example` para
  empezar.
- Grupos de variables principales:
  - **Proveedores**: claves y modelos de OpenAI / Anthropic; host y modelo de
    Ollama.
  - **Selección de modelo**: `DEFAULT_MODEL` (`auto` o etiqueta explícita).
  - **Conocimiento**: ruta del store, proveedor y modelo de embeddings.
  - **Confiabilidad**: reintentos, timeout, backoff.
  - **Salida estructurada**: reintentos de validación de esquema.
  - **Pipeline de actividad**: ruta de la base SQLite, intervalo de sondeo.
  - **Logging**: on/off, nivel, directorio.
- Valores mal formados o vacíos **caen al default** en silencio, no fallan.

> **Nota del orador:** Mensaje: "arrancar es copiar un archivo y poner una
> clave (o apuntar a Ollama)". Bajo umbral de entrada.

---

## 22. Estado del proyecto y cierre

**Título de slide:** Estado y próximos pasos

- **Fase 1 (núcleo)**: completa — modelo, herramientas, memoria, bucle ReAct,
  salida estructurada, multi-agente.
- **Fase 2 (conocimiento)**: completa — embeddings, índice SIDX, extracción de
  documentos, integración con el agente.
- **Fase 3 (superficie)**: completa — MCP, planner, TUI, CLI. El binario
  compila y corre punta a punta contra un Ollama local real.
- **Fase 4 (pipeline de actividad)**: completa — eventos, privacidad,
  actividad, hábitos, semántico, sensores (interfaz); test end-to-end limpio
  bajo `-race`.
- **Trabajo futuro documentado**: sensores concretos de macOS (proceso satélite
  Swift), autocompletado en la TUI, posible backend ANN para el índice.

**Cierre sugerido:**
- Simon = **agente + RAG + actividad local-first**, en un binario Go liviano,
  sin CGO y con privacidad por diseño.
- Cada funcionalidad tiene un **ejemplo ejecutable** que sirve como referencia.

> **Nota del orador:** Cerrar invitando a correr `basic_agent` y
> `activity_pipeline_example` para ver las dos puntas del SDK.

---

## Apéndice A — Mapa rápido "funcionalidad → dónde vive"

| Funcionalidad | Paquete(s) | Ejemplo(s) |
|---|---|---|
| Agente / bucle ReAct | `internal/agent` | `basic_agent` |
| Salida estructurada | `internal/agent` (`RunStructured`) | `structured_output_agent` |
| Observabilidad / eventos | `internal/agent` (`WithOnEvent`) | `hooks_agent` |
| Herramientas | `internal/tool` | `builtin_tools_agent`, `tool_runner_example` |
| Estado por request | `internal/tool` + closures | `run_context_example` |
| Memoria | `internal/memory` | `memory_agent`, `persistent_memory_agent` |
| Multi-agente | `internal/multi` | `parallel_agents`, `agent_pool_example`, `triage_agent` |
| Proveedores / router | `internal/model`, `internal/router` | (transversal) |
| RAG / conocimiento | `internal/knowledge` (+ embed/index/extract) | `knowledge_agent` |
| CLI | `cmd/simon` | (chat/ask/index/plan) |
| TUI de chat | `internal/tui` | `chat_tui` |
| Planner | `internal/planner` | `planner_agent` |
| Cliente MCP | `internal/mcp` | `mcp_agent` (+ `mcp_agent/server`) |
| Pipeline de actividad | `events`, `privacy`, `semantic`, `activity`, `habits`, `sensors` | `activity_pipeline_example` |
| Errores | `pkg/simonerr` | (transversal) |
| Configuración | `internal/config` | (transversal) |

---

## Apéndice B — Frases-gancho para las slides

- *"Piensa, actúa, observa, repite — hasta responder."* (bucle ReAct)
- *"Tus títulos de ventana nunca salen del dispositivo."* (privacidad)
- *"Un struct de parámetros, cero magia de reflexión de firmas."* (tools)
- *"Errores que son datos para el modelo, no crashes."* (dispatch de tools)
- *"Un binario Go, sin CGO, compilable de forma cruzada."* (portabilidad)
- *"Cada funcionalidad, un ejemplo que corre."* (examples/)
