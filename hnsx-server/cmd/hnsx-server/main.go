// hnsx-server is the HnsX control plane daemon. It hosts the HTTP/REST API
// and the gRPC control plane for Python Runtime Workers.
//
// Usage:
//
//	hnsx-server server [--config <path>]
//	hnsx-server version
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/hnsx-io/hnsx/server/internal/bootstrap"
	"github.com/hnsx-io/hnsx/server/pkg/version"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	switch os.Args[1] {
	case "server":
		os.Exit(runServer(os.Args[2:]))
	case "version", "--version", "-v":
		fmt.Println(version.String())
	default:
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Print(`hnsx-server — HnsX Control Plane

Usage:
  hnsx-server server [--config <path>]
  hnsx-server version

Environment:
  HNSX_HTTP_ADDR           Listen address (default 127.0.0.1:50051)
  HNSX_GRPC_ADDR           gRPC listen address (default 127.0.0.1:50061)
  HNSX_DATABASE_URL        Postgres connection string. When empty, embedded
                           SQLite is used (see HNSX_DAEMON_DATA_DIR).
  HNSX_DAEMON_DATA_DIR     Where SQLite DB and secret.key live in daemon mode
                           (default ~/.local/share/hnsx)
  HNSX_SQLITE_PATH         Override the SQLite file path
  HNSX_MIGRATIONS_DIR      SQL migrations directory
  HNSX_OTEL_EXPORTER       stdout | otlp | none (default otlp)
  HNSX_OTEL_OTLP_ENDPOINT  OTLP gRPC endpoint (default 127.0.0.1:4317)
  HNSX_OTEL_SERVICE_NAME   service.name attribute
  HNSX_LOG_LEVEL           debug | info | warn | error
  HNSX_REDIS_ADDR          Redis address for the session queue (e.g. 127.0.0.1:6379)
  HNSX_REDIS_PASSWORD      Redis AUTH password
  HNSX_REDIS_DB            Redis logical database number
  HNSX_REDIS_QUEUE_PREFIX  Redis key prefix for the queue (default hnsx:queue)
`)
}

func runServer(args []string) int {
	srv, err := bootstrap.NewServerFromArgs(args)
	if err != nil {
		log.Fatalf("bootstrap: %v", err)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	if err := srv.Run(ctx); err != nil && !bootstrap.IsCleanShutdown(err) {
		log.Fatalf("server: %v", err)
	}
	return 0
}
