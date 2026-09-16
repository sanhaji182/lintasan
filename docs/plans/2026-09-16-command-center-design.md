# Lintasan Command Center — Phase 1 Design

Status: Approved direction, implementation baseline
Date: 2026-09-16

## Goal

Turn the dashboard from a long feature index into a calm operator workspace. The baseline makes the two highest-frequency journeys obvious: understand gateway state from Overview, then manage provider accounts and models from Connections.

## Current-state inventory

- Shell: fixed 264px sidebar plus 64px header; route title is present but there is no cross-product navigation search.
- Information architecture: Primary / Gateway / Observability / Manage / Advanced exposes 21 destinations and uses implementation-oriented group names.
- Overview: reads `/api/dashboard/stats`, `/api/logs`, and `/api/connections`, but silently converts failed sources to zero/empty data. It also renders a fabricated `Tokens Today: 0`, and labels lifetime request totals as “Last 24 hours”.
- Connections: already owns provider/account/model lifecycle, including Safe test diagnostics, provider summaries, account actions, model sync/view/test, OAuth IDE, curl import, pools, balances, presets, and bulk actions. Its progressive provider disclosure is a strong base, but the page header and summary hierarchy compete with the global shell.
- Models: callable catalog and model-level testing remain a focused downstream inspection surface.
- Routing: policy/combo/alias/quota editing has explicit scoped save boundaries and dirty guards.
- Analytics/Logs: canonical analytics route already consolidates observability tabs; legacy routes are related to it in navigation.
- Settings: broad configuration remains available but should not dominate daily operation.
- Mobile: sidebar becomes a drawer at 768px and main padding contracts, but page-level wide tables remain internally scrollable. The shell needs stronger width containment and 44px touch targets.

## Information architecture

Preserve every canonical route while regrouping navigation by intent:

1. Operate — Overview, Connections, Routing
2. Build — Quickstart, Playground, Models, Provider Catalog
3. Observe — Analytics
4. Configure — API Keys, Teams, Users, Webhooks, Settings, and advanced tools

Operate, Build, and Observe are open by default. Configure is collapsed unless its route is active. Legacy nested intent routes continue to activate their canonical parent (`discover`/`oauth-ide` → Connections, `fallback` → Routing, usage/savings/logs/observability → Analytics). No backend route changes are required.

## Shell and command palette

- Sidebar uses stronger context branding (“Command Center”), grouped destinations, status-safe active styling, and a full-width accessible theme control.
- Header shows the current section and a navigation-only command trigger. `⌘K`/`Ctrl+K` and `/` open a modal search; Escape closes it and restores focus. Results are only real routes from the navigation model—no fake operational commands.
- Desktop keeps the sidebar; tablet/mobile uses a modal drawer and compact search trigger.

## Overview truth model

Fetch stats, logs, connections, and `/health` independently. Each source owns its loading/error/ready state. A source failure is never rewritten as zero.

- Gateway status comes only from `/health`; unavailable means “Status unavailable”, not unhealthy.
- Request count and latency come only from dashboard stats and are described as recorded totals/average, not a fabricated time window.
- Token throughput is derived only from returned log rows and labeled as the visible recent sample.
- Provider readiness is derived only from returned connections and explicitly reports active/total.
- Partial failures produce inline source notices while healthy panels remain usable.
- Empty states link to real actions: add a connection, open Quickstart, inspect Analytics.

## Connections baseline

Keep all existing contracts and Safe-test/NousResearch behavior. Treat provider summary rows as the default scan layer and account/model details as progressive disclosure. Retain one primary page CTA (`Add`), keep bulk mode secondary, and add concise context copy plus consistent summary surfaces. No credential or provider data is invented.

## Visual system

A Linear/Vercel-inspired system with intentional light and dark palettes:

- Cool neutral canvas, crisp elevated surfaces, restrained indigo accent.
- Semantic success/warning/error/info tints only for real states.
- 8px spacing rhythm, 6–12px functional radii, low-noise borders rather than decorative shadows.
- Inter/system sans and JetBrains/system mono, compact headings, readable 13–16px UI type.
- 44px minimum interactive targets on touch layouts, visible focus rings, reduced-motion fallbacks.
- `min-width: 0`, responsive grids, and bounded overlays prevent shell-level horizontal overflow.

## Accessibility and responsive behavior

- Dialog semantics, labeled search, keyboard opening/closing, focus transfer/restoration.
- Navigation uses `aria-current`; collapsible groups expose `aria-expanded`.
- Sidebar overlay and command palette are independently dismissible.
- At ≤768px sidebar is off-canvas, header labels reduce, cards stack, and primary actions remain reachable.
- At ≤520px command result metadata and nonessential labels collapse; no page shell overflow.

## Testing strategy

1. Pure navigation tests enforce route preservation, intent groups, related-route activation, and searchable destinations.
2. DOM tests exercise command palette keyboard behavior, focus, filtering, and real navigation links.
3. Overview DOM tests prove partial-source failures are visible and do not fabricate zero metrics.
4. Existing Connections workflow tests guard progressive disclosure and Safe-test behavior.
5. Run frontend test/check/build and relevant Go dashboard/server tests.
6. Browser QA at desktop and mobile widths; use a local authenticated test token/session only, without mutating production.
