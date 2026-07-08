# Alice AI Gateway Electron Desktop Wrapper

This directory contains the Electron wrapper for Alice AI Gateway. It is
retained from the upstream New API desktop packaging path and is not the primary
OpenAlice production deployment target.

OpenAlice production should normally run Alice AI Gateway as a server-side data
plane controlled by OpenAlice Cloud. Use this wrapper only when a local desktop
packaging smoke test is useful.

## Build

From this directory:

```bash
./build.sh
```

The script builds the frontend, builds the Go binary as `alice-ai-gateway`, and
then runs the platform-specific Electron build.

## Development

If you already have a pre-built binary:

```bash
cp ../alice-ai-gateway-macos ../alice-ai-gateway
npm install
npm run dev-app
```

The wrapper stores local SQLite data in `alice-ai-gateway.db`.

## Attribution

This wrapper remains part of a modified New API distribution. Preserve
`LICENSE`, `NOTICE`, and `THIRD-PARTY-LICENSES.md` in packaged builds.
