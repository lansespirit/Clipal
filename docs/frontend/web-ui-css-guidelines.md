# Web UI CSS Guidelines

## Purpose

These rules keep the Clipal Web UI safe to evolve.

The main failure mode we are preventing is semantic leakage:

- a class name says one thing
- multiple pages reuse it for unrelated layouts
- a small visual change causes regressions across tabs

## Core Rules

### 1. Use page-scoped blocks for page layout

Page sections must own their own block names.

Examples:

- `provider-card__header`
- `status-card__metrics`
- `takeover-page__header`
- `service-panel__copy`

Do not reuse another page's block class just because the flex layout looks similar.

### 2. Use primitives for visual language

Use primitives for reusable shapes and tones:

- `pill`
- `badge`
- `chip`

Prefer modifiers over one-off aliases.

Examples:

- `pill pill--xs pill--neutral pill--mono`
- `badge badge--xs badge--outline badge--mono`
- `chip chip--primary`

### 3. Shared classes must not own page rhythm

A shared primitive may define:

- shape
- internal padding
- tone
- typography treatment

A shared primitive must not define:

- section bottom spacing
- page-specific borders
- page-specific horizontal alignment

### 4. Keep block naming lightweight BEM

Use:

- block: `status-card`
- element: `status-card__header`
- modifier: `status-card__header--compact`
- state: `is-disabled`

### 5. Avoid misleading generic names

Do not add new names like:

- `provider-header`
- `status-metrics`
- `detail-item`

unless the selector truly belongs to a shared design component used intentionally across the whole UI.

## Primitive Rules

### Pill

Use for passive inline capsules.

Common modifiers:

- `pill--xs`
- `pill--sm`
- `pill--compact`
- `pill--neutral`
- `pill--mono`
- `pill--primary`
- `pill--success`
- `pill--warning`
- `pill--danger`

### Badge

Use for compact markers attached to a title or entity.

Common modifiers:

- `badge--xs`
- `badge--sm`
- `badge--outline`
- `badge--warning`
- `badge--mono`

### Chip

Use for repeated member entities in grouped lists.

Common modifiers:

- `chip--primary`
- `chip--muted`
- `chip--danger`

## File Ownership

- `tokens.css`: variables only
- `base.css`: reset, base typography, utilities, keyframes
- `primitives.css`: pill, badge, chip, primitive modifiers
- `components.css`: temporary compatibility layer until legacy selectors are fully migrated
- `pages.css`: page-scoped blocks

## Review Checklist

Before merging Web UI CSS work, verify:

- the class name matches the actual ownership
- the change only affects the intended page or block
- a primitive is not being used as page layout glue
- no new one-off alias was introduced when a modifier would do
- the tab was checked in the browser on `3433`

## Anti-Patterns

Do not:

- reuse another page's header class
- add DOM-structure tests for exact visual layout
- create new aliases like `foo-pill` when `pill--*` modifiers are enough
- put page spacing onto a primitive selector

## Migration Note

`index.html` now loads the layered stylesheets directly, and `components.css` no longer imports the legacy `styles.css`.

New work should prefer:

- page-scoped block selectors in `pages.css`
- primitive modifiers in `primitives.css`

and should avoid adding new selectors to legacy `styles.css` unless there is a deliberate temporary compatibility reason.
