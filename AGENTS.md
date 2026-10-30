## Module layout

`cmd` holds entrypoints only. `internal` holds all private code. Shared contracts other modules import live at the module root as versioned subpackages, never under `internal`.

## Dependency rule

Imports point inward only. Handlers call services. Services call repositories or brokers. Nothing calls upward. A function needing five unrelated dependencies is a design error, not a feature.

## Package rule

One package per domain concept. Split into a subpackage when a consumer needs only part of it. Group by what changes together, not by file type. Parent package is wiring. Child package is behavior.

## Anti-restructure clause

Do not reorganize packages, rename files for style, or introduce new abstraction layers unless explicitly asked. Generate code inside the existing boundaries. If a required name is missing during a requested refactor, propose your best names and continue — do not stop to ask.

## Package naming

Every package is named after a real thing in the problem domain — a business process, an entity, or a subsystem. Use verb-noun or noun forms that match how a domain expert talks about the work. Never a bare verb, and never a generic label like util, common, or core — except when the term itself names a real architectural layer, such as api for the HTTP-serving boundary. If you cannot name the package after something concrete from the domain, stop and ask before creating it.

## Artifact placement

Artifacts belong next to the code they describe, not at the project root. Root is reserved for artifacts that span the whole project — docs, docker-compose, top-level README — and for any path required by a tool, platform, or language runtime, such as Postman collections, CI configs, Dockerfiles, go.mod, OpenAPI specs, or generated code output directories. The test: if the location is dictated by your architecture, co-locate. If it’s dictated by someone else’s contract, the tool wins. If unsure whether something qualifies as exempt, ask before moving it. Schema migrations live in exactly one ordered directory inside the engine adapter that owns them (e.g. `internal/store/postgres/migrations`), never at the project root and never split across packages.

## Function decomposition

Every function has one responsibility. Orchestrating a sequence of calls to smaller helpers is one responsibility, not a violation. The violation is a function that validates, transforms, persists, and notifies inline. If a function exceeds roughly twenty lines or mixes multiple stages, treat that as a smell and extract each stage into a named function at the same abstraction level. The extracted name must say what it does without reading the body. Dead code left behind by an extraction is a defect, not cleanup for later.

## Error handling

Every function handles both its success path and its failure routes completely. No path is an afterthought. Errors are returned, logged, or translated at the layer that owns them, never swallowed or ignored. Do not use the phrase “happy path” in code, comments, or conversation. All paths are intentional and all are specified.

## Adapters

Infrastructure is accessed only through interfaces. Domain packages depend only on those interfaces. They never import concrete adapters and never contain store, repository, SQL, migration, or backend-specific files. `cmd` is the only place that constructs concrete adapters and injects interfaces (or a dispatcher that holds multiple adapters).

Use the collapsed form by default: the interface and its single implementation live in the same parent package (`internal/store`, `internal/broker`, `internal/object`). Expand to the full hierarchy — interfaces in the parent, concrete engines or backends in children (`internal/store/postgres`, `internal/object/gcs`, `internal/object/s3`, etc.) — only when more than one implementation exists or is planned, or when `cmd` would otherwise hardcode an engine. All SQL, queries, and migrations for a given engine live only in that engine’s adapter package. The same applies to object storage and brokers.

Do not create packages named records, bus, or any other infrastructure label. Use only store, broker, and object unless explicitly told otherwise. Do not put CRUD or storage operations in domain packages. They belong in the concrete adapter that implements the interface.

When multiple backends must be selected per call (for example during a migration or when half the documents live in GCS and half in S3), introduce a thin dispatcher that holds the adapters and routes by key, config, or an explicit argument. Domains still call only the interface. `cmd` builds the adapters and the dispatcher; it does not make the per-call choice.