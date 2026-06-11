# GHO-43 Frontend Regression Report

## Scope

- Added admin settings UI for `download_max_retry` and `scrape_full_url`.
- Added Japanese validation and success/error feedback for settings save and default restore.
- Added zip/csv drag-and-drop upload UI for full sync import.
- Added upload result display and sync history refresh after successful upload.
- Updated frontend API client types and methods for `/admin/settings` and `/sync/upload`.

## Business Rules Covered

- Retry count must be an integer from 0 to 10.
- Full scrape URL must use `https` and the Japan Post official domain.
- Restore default sends `reset_to_default` for both configurable settings.
- Upload accepts only `.zip` or `.csv` files on the client side.
- Upload responses display success/failure status, row counts, and structured API errors in Japanese.
- Successful upload refreshes sync status and sync run history.

## Verification

- `npm test -- --run` in `web/`: 15 tests passed.
- `npm run build` in `web/`: TypeScript check and Vite production build passed.
- `GOCACHE=/tmp/jdps-go-build go test -p 1 ./...`: passed.

## Notes

- `go test ./...` without `-p 1` hit a Go compiler SIGSEGV while compiling `net/http` in this environment. The same suite passed when rerun serially with an explicit writable `GOCACHE`.
