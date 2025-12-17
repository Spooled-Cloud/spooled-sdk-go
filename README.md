# Spooled Go SDK

Official Go SDK for [Spooled Cloud](https://spooled.cloud) — a modern, scalable job queue and task scheduler.

## Status: 🚧 Coming Soon

The Go SDK is currently in development. For now, you can use:

- **REST API** — Works with any HTTP client (`net/http`, `resty`, etc.)
- **gRPC API** — Use the proto file directly with `google.golang.org/grpc`

## Using the REST API

```go
package main

import (
    "bytes"
    "encoding/json"
    "fmt"
    "net/http"
)

func main() {
    apiKey := "sk_live_your_api_key"
    baseURL := "https://api.spooled.cloud"

    // Create a job
    payload := map[string]interface{}{
        "queue_name": "emails",
        "payload": map[string]string{
            "to":      "user@example.com",
            "subject": "Hello!",
        },
        "max_retries": 3,
    }

    body, _ := json.Marshal(payload)
    req, _ := http.NewRequest("POST", baseURL+"/api/v1/jobs", bytes.NewBuffer(body))
    req.Header.Set("Authorization", "Bearer "+apiKey)
    req.Header.Set("Content-Type", "application/json")

    resp, err := http.DefaultClient.Do(req)
    if err != nil {
        panic(err)
    }
    defer resp.Body.Close()

    var result map[string]interface{}
    json.NewDecoder(resp.Body).Decode(&result)
    fmt.Printf("Created job: %s\n", result["id"])
}
```

## Using the gRPC API

```bash
# Download the proto file
curl -O https://raw.githubusercontent.com/spooled-cloud/spooled-backend/main/proto/spooled.proto

# Generate Go code
protoc --go_out=. --go-grpc_out=. spooled.proto
```

```go
package main

import (
    "context"
    "log"

    "google.golang.org/grpc"
    "google.golang.org/grpc/credentials"
    "google.golang.org/grpc/metadata"

    pb "your-module/spooled"
)

func main() {
    // Connect to Spooled Cloud
    creds := credentials.NewClientTLSFromCert(nil, "")
    conn, err := grpc.Dial("grpc.spooled.cloud:443", grpc.WithTransportCredentials(creds))
    if err != nil {
        log.Fatal(err)
    }
    defer conn.Close()

    client := pb.NewQueueServiceClient(conn)

    // Add API key to context
    ctx := metadata.AppendToOutgoingContext(context.Background(), "x-api-key", "sk_live_your_api_key")

    // Enqueue a job
    resp, err := client.Enqueue(ctx, &pb.EnqueueRequest{
        QueueName:  "emails",
        Payload:    `{"to": "user@example.com"}`,
        Priority:   5,
        MaxRetries: 3,
    })
    if err != nil {
        log.Fatal(err)
    }

    log.Printf("Created job: %s", resp.JobId)
}
```

## Coming Features

- [ ] Idiomatic Go client with interfaces
- [ ] Generics support for typed payloads
- [ ] Context propagation
- [ ] Connection pooling
- [ ] Worker runtime
- [ ] Comprehensive test suite
- [ ] Benchmarks

## Contributing

Interested in contributing? See [CONTRIBUTING.md](https://github.com/spooled-cloud/spooled-backend/blob/main/CONTRIBUTING.md).

## License

Apache 2.0
