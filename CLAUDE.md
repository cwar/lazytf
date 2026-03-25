# CLAUDE.md — lazytf

## Project

lazytf is a lazygit-style TUI for Terraform, built with Go and Bubble Tea.

## Build & Test

```bash
go build -o lazytf ./cmd/lazytf/    # build binary
go test ./...                        # run all tests
go test -race ./...                  # run with race detector
go test ./internal/tui/ -run TestFoo # run specific test
```

The binary is symlinked from `~/.local/bin/lazytf` → `./lazytf`. Rebuild after changes.

## Code Layout

```
cmd/lazytf/           Entry point, arg parsing, stale plan cleanup
internal/terraform/   Runner (wraps terraform CLI), modules parser, graph parser
internal/tui/         Bubble Tea model, key handling, panels, overlays
internal/ui/          Styles, HCL/plan highlighting, ANSI-aware line wrapping
```

## Key Patterns

- **Multi-workspace mode**: `W` key triggers parallel plan/apply across workspaces. Uses `TF_WORKSPACE` env var per-subprocess (no workspace switching needed). Config via `.lazytf.yaml` (ignore list + named groups). Semaphore limits concurrency to 4. State lives in `multiWSState` struct inside Model.
- **Bubble Tea value receivers**: `Update`, `View`, and key handlers use value receivers (`func (m Model)`) — this is correct for Bubble Tea. Helper methods that don't return the model use pointer receivers (`func (m *Model)`).
- **Streaming commands**: Plan/Apply/Destroy use `runTfCmdStream` with channel-based line streaming. Non-streaming commands use `runTfCmd` with `tea.Sequence`. Both support context cancellation.
- **Safe apply flow**: `a` → plan with `-out=file` → review mode → `y` to apply. No `-auto-approve`.
- **Busy guard**: `busyguard.go` prevents concurrent terraform operations. All new operation keys must be registered in `isOperationKey()` or `isContextOperationKey()`.
- **Resource file index**: Built during `loadAllData`, provides O(1) lookup in `findResourceFile`. Rebuilt on every data reload.

## Testing Conventions

- Test files live alongside source (`foo.go` → `foo_test.go`)
- Test helpers: `testModel()`, `baseBusyModel()`, `basePlanReviewModel()`, `sendKey()`, `sendSpecialKey()`
- Use `t.TempDir()` for any file I/O in tests
- Run `go test -race ./...` before committing — the parallel `loadAllData` makes race detection important

---

## Incremental Model Extraction Plan

The `Model` struct in `model.go` is a god-object (~40 fields). It works correctly but makes reasoning about state transitions harder as features grow. **Extract sub-structs incrementally** — one cluster at a time, not all at once.

### Extraction Order (each is an independent PR)

#### 1. Plan Review State → `planState`

Extract these fields from Model into a `planState` struct:

```go
type planState struct {
    file        string       // pendingPlanFile
    active      bool         // planReview
    isDestroy   bool         // planIsDestroy
    changes     []planChange // planChanges
    changeCur   int          // planChangeCur
    focusView   bool         // planFocusView
    compactDiff bool         // planCompactDiff
    compactLines      []string
    compactHighlighted []string
}
```

**Access pattern**: `m.plan.active` instead of `m.planReview`.

**Files to update**: `keys.go` (plan review key handling), `update.go` (cmdDoneMsg plan entry), `view.go` (renderDetailPane, renderHelpHint), `planrecall.go` (save/restore), `busyguard.go` (no change — doesn't reference plan fields).

**Test files**: `planscroll_test.go`, `planrecall_test.go`, `applyresult_test.go`, `clipboard_test.go`.

#### 2. Plan Recall State → `savedPlan`

Extract these fields:

```go
type savedPlan struct {
    file        string
    isDestroy   bool
    lines       []string
    highlighted []string
    changes     []planChange
    title       string
}
```

**Access pattern**: `m.saved.file` instead of `m.lastPlanFile`. Methods `savePlanState`, `restorePlanState`, `clearLastPlan`, `hasLastPlan` move to `savedPlan` methods.

**Files to update**: `planrecall.go` (becomes methods on savedPlan), `keys.go` (R key, esc in plan review), `view.go` (renderHelpHint recall hint), `model.go` (clearLastPlan calls).

**Test files**: `planrecall_test.go`.

#### 3. Overlay State → `overlayState`

Extract:

```go
type overlayState struct {
    help    bool
    log     bool
    graph   bool
    confirm bool
    confirmAction string
    confirmMsg    string
    confirmData   string
    input   bool
    inputPrompt string
    inputValue  string
    inputAction string
}
```

**Access pattern**: `m.overlay.help` instead of `m.showHelp`.

**Files to update**: `keys.go` (overlay dismissal, confirm, input checks), `contextkeys.go` (showInput, showConfirm), `view.go` (View, renderConfirm, renderInput, renderHelp).

**Test files**: `busyguard_test.go` (doesn't reference overlays directly — safe).

### Rules for Each Extraction

1. **One sub-struct per PR** — don't combine extractions.
2. **Mechanical rename first** — change `m.planReview` → `m.plan.active` everywhere. No behavior changes.
3. **Run `go test -race ./...` after** — the struct copy semantics in Bubble Tea value receivers must still work correctly. Sub-structs are copied by value (which is what we want).
4. **Move methods second** — after the rename PR merges, move related methods to be receivers on the sub-struct in a follow-up.
5. **Update memories** — after each extraction, update the stored pi memories (`lazytf-architecture`, `lazytf-context-keys`, etc.) to reflect the new field paths.
