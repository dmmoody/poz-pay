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
