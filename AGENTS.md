# poz-pay

## Module layout

`cmd` holds entrypoints only. `internal` holds all private code. Shared contracts other modules import live at the module root as versioned packages, never under `internal`.

## Dependency rule

Imports point inward only. Handlers call services. Services call repositories or brokers. Nothing calls upward. A function needing five unrelated dependencies is a design error, not a feature.

## Package rule

One package per domain concept. Split into a subpackage when a consumer needs only part of it. Group by what changes together, not by file type. Parent package is wiring. Child package is behavior.

## Anti-restructure clause

Do not reorganize packages, rename files for style, or introduce new abstraction layers unless explicitly asked. Generate code inside the existing boundaries.

## Package naming

Every package is named after a real thing in the problem domain – a business process, an entity, or a subsystem. Use verb-noun or noun forms that match how a domain expert talks about the work. Never a bare verb, and never a generic label like util, common, or core – except when the term itself names a real architectural layer, such as api for the HTTP-serving boundary. If you cannot name the package after something concrete from the domain, stop and ask before creating it.

## Artifact placement

Artifacts belong next to the code they describe, not at the project root. Root is reserved for artifacts that spane the whole project – docs, docker-compose, top-level README – and for any path required by a tool, platform, or language runtime, such as Postman collections, CI configs, Dockerfiles, go.mod, OpenAPI specs, or generated code output directories. The test: if the location is dictated by someone else's contract, the tool wins. If unsure, whether something qualifies as exempt, ask before moving it.

## Function decomposition

Every function has one responsibility. Orchestrating a sequence of calls to smaller helpers is one responsibility, not a violation. The violation is a function that validates, transforms, persists, and notifies inline. If a function exceeds roughly twenty lines or mixes multiple stages, treat that as a smell and extract each stage into a named function at the same abstraction level. the extracted name must say what it does without reading the body. Dead code left behind by an extraction is a defect, not cleanup for later.

## Error handling

Every function handles both its success path and its failure path routes completely. No path is an afterthought. Errors are returned, logged, or translated at the layer that owns them, never swallowed or ignored. Do not use the phrase "happy path" in code, comments, or conversation. All paths are intentional and all are specified.
