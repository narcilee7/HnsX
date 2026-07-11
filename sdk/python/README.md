# hnsx (Python SDK)

Python SDK for the HnsX Harness platform.

## Install

```bash
pip install -e "sdk/python"
```

## Usage

### REST client

```python
from hnsx import HnsXClient

client = HnsXClient("http://127.0.0.1:50052")
session = client.sessions.trigger(
    domain_id="customer-service",
    trigger={"question": "I want a refund"},
)
print(session.id, session.state)
```

### SSE stream

```python
for event in client.stream_session_events(session.id):
    print(event.name, event.payload)
```

### DomainSpec builder

```python
from hnsx import DomainSpecBuilder

spec = (
    DomainSpecBuilder("my-agent", description="A simple agent")
    .with_agent("assistant", provider="openai", model="gpt-4o-mini", system_prompt="default")
    .with_prompt("default", "You are a helpful assistant.")
    .with_session_mode("single", agent="assistant")
    .with_policy(max_cost_usd=1.0, max_turns=20)
    .build()
)
```

## Development

```bash
cd sdk/python
pip install -e ".[dev]"
pytest
```
