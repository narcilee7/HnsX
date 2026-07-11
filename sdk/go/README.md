# HnsX Go SDK

Go SDK for the HnsX Harness platform REST API.

## Install

```bash
go get github.com/hnsx-io/hnsx/sdk/go
```

## Usage

```go
package main

import (
	"context"
	"log"

	"github.com/hnsx-io/hnsx/sdk/go"
)

func main() {
	client := hnsx.NewClient("http://127.0.0.1:50052")

	session, err := client.Sessions.Trigger(context.Background(), "customer-service", map[string]any{
		"question": "I want a refund",
	})
	if err != nil {
		log.Fatal(err)
	}
	log.Println(session.ID, session.State)
}
```

## Resources

- `client.Domains` — list, get, register YAML
- `client.Sessions` — list, get, trigger, cancel
- `client.Traces` — list, get
- `client.Approvals` — list, approve, reject
- `client.Evals` — list sets, create, run

## Development

```bash
cd sdk/go
go test ./...
```
