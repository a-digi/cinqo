---
name: backend-feature
description: Use when building a new backend feature end-to-end — orchestrates design, implementation, and review with fix loops until the reviewer approves.
---

# Backend Feature Pipeline

Orchestrates a backend feature from idea to approved code through three roles: Architect → Developer → Reviewer, with a fix loop until the reviewer has no blocking issues.

## Pipeline

```dot
digraph backend_feature {
    "Understand feature request" [shape=box];
    "ARCHITECT: Design" [shape=box];
    "Present design to user" [shape=box];
    "User approves?" [shape=diamond];
    "DEVELOPER: Implement" [shape=box];
    "REVIEWER: Review" [shape=box];
    "Blocking issues?" [shape=diamond];
    "DEVELOPER: Fix issues" [shape=box];
    "Done" [shape=doublecircle];

    "Understand feature request" -> "ARCHITECT: Design";
    "ARCHITECT: Design" -> "Present design to user";
    "Present design to user" -> "User approves?";
    "User approves?" -> "ARCHITECT: Design" [label="no, revise"];
    "User approves?" -> "DEVELOPER: Implement" [label="yes"];
    "DEVELOPER: Implement" -> "REVIEWER: Review";
    "REVIEWER: Review" -> "Blocking issues?";
    "Blocking issues?" -> "DEVELOPER: Fix issues" [label="yes"];
    "DEVELOPER: Fix issues" -> "REVIEWER: Review";
    "Blocking issues?" -> "Done" [label="no"];
}
```

## Step-by-Step

### Step 1 — Understand the request
Read the feature request carefully. If anything is ambiguous (which resource, which scopes, schema changes, whether a migration is needed), ask ONE clarifying question before proceeding. Do not ask multiple questions at once.

### Step 2 — ARCHITECT: Design
Invoke the `senior-backend-architect` skill.

Produce a design covering:
- API shape (method, path, scope, request body, response)
- Data model (new tables or columns with types and constraints)
- Package location (entity, query repo, persistent repo, handler)
- Scope requirements (existing scope or new one with justification)
- Migration SQL
- Security considerations

### Step 3 — Present design to user
Show the design clearly. Wait for explicit approval before writing any code. If the user requests changes, revise the design and present again.

### Step 4 — DEVELOPER: Implement
Invoke the `senior-backend-developer-golang` skill.

Implement the approved design exactly. Follow the implementation checklist in that skill (entity → repos → handler → register resource → add route → migration). Do not add features not in the design.

### Step 5 — REVIEWER: Review
Invoke the `code-reviewer-backend` skill.

Review all changed files against the security, pattern compliance, and Go quality checklists. Produce a findings report in the format defined by that skill.

### Step 6 — Fix loop
If the reviewer output is `CHANGES REQUIRED`:
- Re-invoke `senior-backend-developer-golang`
- Address **every BLOCKING finding** — one by one, confirm each is resolved
- Re-invoke `code-reviewer-backend` on the changed files
- Repeat until the reviewer output is `APPROVED`

### Step 7 — Done
Report to the user:
- What was built (files created/changed, route added, migration file name)
- Any ADVISORY findings left open (with explanations)
- Confirmation that the reviewer approved

## Rules

- **Never skip the design step.** Implementation without an approved design is not allowed.
- **Never skip the review step.** "It looks fine" is not a review.
- **Fix all BLOCKING issues before reporting done.** Advisory findings may be left open with documentation.
- **Do not change scope mid-implementation.** If implementation reveals the design needs a change, surface it — don't silently expand.
- **Migrations are append-only.** Never edit an existing migration file, even to fix it. Add a new one.
