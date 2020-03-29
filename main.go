package main

import (
	"context"
	"expvar"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/DataDog/datadog-go/statsd"
	"go.uber.org/zap"
	muxtrace "gopkg.in/DataDog/dd-trace-go.v1/contrib/gorilla/mux"
	"gopkg.in/DataDog/dd-trace-go.v1/ddtrace/tracer"
)

func main() {
	tracer.Start()
	defer tracer.Stop()

	logger, _ := zap.NewProduction()
	defer logger.Sync() // flushes buffer, if any
	sugar := logger.Sugar().Named("gophercon")

	sugar.Info("The app is starting...")

	c, err := statsd.New("127.0.0.1:8125")
	if err != nil {
		sugar.Fatalw("Failed to start statsd", "err", err)
	}
	c.Namespace = "gophercon"
	c.Tags = []string{"gophercon-2020"}

	port := os.Getenv("PORT")
	if port == "" {
		sugar.Fatal("PORT is not set")
	}

	diagPort := os.Getenv("DIAG_PORT")
	if diagPort == "" {
		sugar.Fatal("DIAG_PORT is not set")
	}

	r := muxtrace.NewRouter()
	server := http.Server{
		Addr:    net.JoinHostPort("", port),
		Handler: r,
	}

	diagLogger := sugar.With("subapp", "diag_router")
	diagRouter := muxtrace.NewRouter()
	diagRouter.Handle("/debug/vars", expvar.Handler())
	diagRouter.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		err = c.Incr("health_calls", []string{}, 1)
		if err != nil {
			diagLogger.Errorw("Could not increment health_calls", "err", err)
		}
		diagLogger.Info("Health was called")
		w.WriteHeader(http.StatusOK)
	})

	diag := http.Server{
		Addr:    net.JoinHostPort("", diagPort),
		Handler: diagRouter,
	}

	shutdown := make(chan error, 2)

	sugar.Info("Business server is starting...")
	go func() {
		err := server.ListenAndServe()
		if err != nil && err != http.ErrServerClosed {
			shutdown <- err
		}
	}()

	sugar.Info("Diagnostics server is starting...")
	go func() {
		err := diag.ListenAndServe()
		if err != nil && err != http.ErrServerClosed {
			shutdown <- err
		}
	}()

	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt, syscall.SIGTERM)

	select {
	case x := <-interrupt:
		sugar.Infow("The app received a stop signal", "signal", x.String())

	case err := <-shutdown:
		sugar.Errorw("Error from functional unit", "err", err)
	}

	timeout, cancelFunc := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelFunc()

	err = server.Shutdown(timeout)
	if err != nil {
		sugar.Errorw("The business logic is stopped with error", "err", err)
	}

	err = diag.Shutdown(timeout)
	if err != nil {
		sugar.Errorw("The diagnostics logic is stopped with error", "err", err)
	}

	sugar.Info("The app is shut downed")
}
