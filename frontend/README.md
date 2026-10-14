# GoJudge Frontend

Next.js frontend scaffold for GoJudge.

## Requirements

- Node.js `^20.19.0 || ^22.13.0 || >=24.0.0`
- npm

The included `.nvmrc` selects Node.js `22.14.0`.

## Setup

```bash
nvm install
nvm use
npm ci
npm run dev
```

Open [http://localhost:3000](http://localhost:3000).

## Verification

```bash
npm audit --audit-level=high
npm run lint
npm run typecheck
npm run test:run
npm run build
```

`lint` runs ESLint (React/Hooks/TypeScript/accessibility and App Router guards)
and Biome (Next-specific rules). `test:run` includes real lint-contract fixtures
before the application unit tests. The full audit includes development dependencies;
there is no vulnerability allowlist. Rule differences and the ESLint 9 maintenance
limitation are documented in the [security migration guide](../docs/interview/microservices/07-frontend-dependency-security.md).
