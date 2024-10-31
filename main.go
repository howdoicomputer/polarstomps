package main

import (
	"fmt"
	"net/http"
	"strings"
	"time"
	"log/slog"
	"os"
	"context"

	"github.com/alexedwards/scs/v2"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/gomodule/redigo/redis"
	"cloud.google.com/go/storage"
	"google.golang.org/api/option"
	"google.golang.org/api/iterator"
)

var (
	sessionManager *scs.SessionManager
	redisPool *redis.Pool
	logger *slog.Logger
    client *storage.Client
)

func getHandler(w http.ResponseWriter, r *http.Request) {
	visited := sessionManager.GetString(r.Context(), "visited")
	component := page(visited)
	component.Render(r.Context(), w)
}

func postHandler(w http.ResponseWriter, r *http.Request) {
	r.ParseForm()

	conn := redisPool.Get()
	defer conn.Close()

	_, err := redis.Int(conn.Do("INCR", "form_click"))
	if err != nil {
		msg := "error incrementing click value"
		logger.Error(msg, slog.String("error", err.Error()))
		http.Error(w, msg, http.StatusInternalServerError)
		return
	}

	logger.Info("Wrote click value increment to Redis")

	if r.Form.Has("country") {
		country := r.Form.Get("country")
		currentVisited := sessionManager.GetString(r.Context(), "visited")

		if !strings.Contains(currentVisited, country) {
			currentVisited += fmt.Sprintf(",%s", country)
		}

		sessionManager.Put(r.Context(), "visited", currentVisited)
	}

	getHandler(w, r)
}

func main() {
	// Initialized a structured logger
	//
	logger = slog.New(slog.NewJSONHandler(os.Stdout, nil))

	// Let's spit out of a version right away to help
	// with debugging.
	//
	logger.Info("Running v22")

	redisHost := os.Getenv("REDIS_HOST")
	redisPort := os.Getenv("REDIS_PORT")
	redisAuth := os.Getenv("REDIS_AUTH")
	redisAddr := fmt.Sprintf("%s:%s", redisHost, redisPort)

	// Conditionally define the endpoint depending on whether or not the
	// GCS_ENDPOINT is populated.
	//
	var clientOptions []option.ClientOption

	if endpoint := os.Getenv("GCS_ENDPOINT"); endpoint != "" {
		clientOptions = append(clientOptions, option.WithEndpoint(endpoint))
		logger.Info(fmt.Sprintf("Using custom endpoint: %s", endpoint))
	} else {
		logger.Info("Using default Google Cloud endpoint")
	}

	// Create a GCS client
	//
	ctx := context.Background()
	client, err := storage.NewClient(ctx)
	if err != nil {
		logger.Error(
			"Error creating GCS client",
			slog.String("error", err.Error()),
		)
	}

	// To demonstrate that we can connect to GCS we
	// just list the contents of a bucket.
	//
	// We're really just testing out GKE workoad
	// identies so we don't need anything complex.
	//
	// Also, let's make the bucket name configurable as it's
	// likely that we're going to use environment prefixes for
	// bucket names.
	//
	bucketName := os.Getenv("GCS_EXAMPLE_BUCKET_NAME")
	bucket := client.Bucket(bucketName, clientOptions...)

	var names []string
	iter := bucket.Objects(ctx, nil)
	for {
		attrs, err := iter.Next()
		if err == iterator.Done {
			break
		}

		if err != nil {
			logger.Error(
				fmt.Sprintf("Error listing bucket contents for %s", bucketName),
				slog.String("error", err.Error()),
			)
		}

		names = append(names, attrs.Name)
	}

	logger.Info(fmt.Sprintf("Bucket contents: %s: ", strings.Join(names, ", ")))

	const maxConnections = 10
	redisPool = &redis.Pool{
		MaxIdle: maxConnections,
		Dial: func() (redis.Conn, error) {
			c, err := redis.Dial("tcp",
				redisAddr,
				redis.DialUsername("default"),
				redis.DialPassword(redisAuth),
			)
			if err != nil {
				logger.Error(
					fmt.Sprintf("Error dialing TCP for redis host %s", redisAddr),
					slog.String("error", err.Error()),
				)

				return nil, err
			}

			return c, nil
		},
	}

	// Initialize the session.
	//
	sessionManager = scs.New()
	sessionManager.Lifetime = 24 * time.Hour

	external := http.NewServeMux()
	internal := http.NewServeMux()

	// Handle POST and GET requests.
	//
	external.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			postHandler(w, r)
			return
		}

		logger.Info(fmt.Sprintf("Recevied request from %s", r.RemoteAddr))

		getHandler(w, r)
	})

	internal.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		getHandler(w, r)
	})

	// Include the static content.
	//
	external.Handle("/assets/", http.StripPrefix("/assets/", http.FileServer(http.Dir("assets"))))

	// Add metrics endpoint.
	//
	internal.Handle("/metrics", promhttp.Handler())

	// Add the middleware.
	//
	externalWithSessionMiddleware := sessionManager.LoadAndSave(external)
	internalWithSessionMiddleware := sessionManager.LoadAndSave(internal)

	// Start the server.
	//
	logger.Info("external exposed on :8080")
	go http.ListenAndServe(":8080", externalWithSessionMiddleware)
	logger.Info("metrics and health exposed on :9090")
	go http.ListenAndServe(":9090", internalWithSessionMiddleware)

	select {}
}
