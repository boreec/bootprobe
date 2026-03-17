# Boottime

Linux CLI tool that collects boot time measurements from four independent sources
(systemd-analyze, systemd D-Bus, EFI variables, ACPI FPDT) and aggregates them
across reboots into JSONL files for comparison and averaging.

## Build & Run Commands

```bash
go build ./cmd/boottime                      # Build binary
go run ./cmd/boottime -R results.jsonl       # Collect boot times (requires root for ACPI/EFI)
go run ./cmd/boottime -A results.jsonl       # Print JSON average
go run ./cmd/boottime -A -p results.jsonl    # Print formatted table average
go test ./...                                # Run all tests
```

## Package Layout

- `cmd/boottime/` -- CLI entry point, flag parsing. `-R` and `-A` are mutually exclusive.
- `acpi/` -- ACPI FPDT table parser (firmware + loader). Two strategies: sysfs (kernel 5.12+) or raw `/dev/mem`.
- `efi/` -- EFI variable reader (firmware + loader) from `/sys/firmware/efi/efivars/`.
- `systemd/` -- Two methods: `systemd-analyze time` output parsing and D-Bus property queries.
- `model/` -- Core types (`BootTimeRecord`, `BootTimeAccumulator`, `RetrievalMethod`, `BootTimeStage`), serialization, table formatting.
- `exec/` -- Orchestration: concurrent retrieval via `errgroup`, JSONL file I/O, averaging.

## Code Conventions

- **Error handling**: Always return `(value, error)`. Wrap errors with `fmt.Errorf("context: %w", err)` to preserve the chain.
- **Naming**: Each retrieval package defines its own `BootTimeRecord` struct scoped to what that source provides. The central `model.BootTimeRecord` maps `BootTimeStage -> RetrievalMethod -> time.Duration`.
- **Concurrency**: `exec.RetrieveBootTimes` runs all four retrieval methods concurrently via `errgroup.Group`.
- **Testing**: Table-driven tests with named cases (`map[string]struct{}`), `t.Parallel()`, `testify/require` for fatal assertions and `testify/assert` for value checks.
- **Data format**: JSONL (one JSON object per line). Durations are nanosecond integers (Go `time.Duration` default JSON marshaling).

## Key Rules

- This is a Linux-only tool. All retrieval methods depend on Linux-specific paths (`/sys/firmware/`, `/dev/mem`) and systemd.
- Do not add a `Makefile` or CI config unless explicitly asked -- these are tracked in TODO.md as future work.
- The `.gitignore` excludes `*.jsonl` files. The tracked `.jsonl` files in the repo root are intentional sample data.
- Keep dependencies minimal. Current direct deps: `godbus/dbus`, `testify`, `golang.org/x/sync`.
