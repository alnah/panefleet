## Summary

- 

## Validation

- [ ] `go test ./...`
- [ ] changed paths are covered by tests

## Go-First Migration Checklist (Required)

- [ ] I did not add new business state logic in shell (`bin/panefleet`, `lib/panefleet/state/engine.sh`).
- [ ] Any state rule change is implemented in `internal/state`.
- [ ] Reducer/projection tests were added or updated for the state rule change.
- [ ] If shell and Go behavior differ, this PR aligns shell behavior to Go behavior.

