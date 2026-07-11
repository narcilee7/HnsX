# @hnsx/sdk-node

Node/TypeScript SDK for the HnsX Harness platform.

## Install

```bash
npm install @hnsx/sdk-node
```

## Usage

### REST client

```ts
import { HnsXClient } from '@hnsx/sdk-node/client'

const client = new HnsXClient('http://127.0.0.1:50052')

// Trigger a session
const session = await client.sessions.trigger({
  domainId: 'customer-service',
  trigger: { question: 'I want a refund' },
})

// Tail the live event stream
for await (const ev of client.streamSessionEvents(session.id)) {
  console.log(ev.name, ev.payload)
}
```

### Proto types

```ts
import { DomainSpecSchema, toJson, fromJson } from '@hnsx/sdk-node'
import type { DomainSpec } from '@hnsx/sdk-node'
```

## Resources

- `client.domains` — list, get, register, update, validate, delete domains
- `client.sessions` — list, get, trigger, cancel, rerun sessions
- `client.traces` — list and get traces
- `client.approvals` — list, get, approve, reject approvals
- `client.evals` — list sets, create, update, run, get runs

## Development

```bash
pnpm install
pnpm type-check
pnpm test
```
