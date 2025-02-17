package server

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"io"
	"net/http"
	"time"
	"fmt"

	"github.com/didip/tollbooth/v7"
	"github.com/didip/tollbooth_chi"
	"github.com/go-chi/chi/v5"
	log "github.com/go-pkgz/lgr"
	"github.com/go-pkgz/rest"
	store "github.com/nilBora/updater/app/store"
	"github.com/nilBora/updater/app/task"
	"github.com/google/uuid"
)

//go:generate moq -out mocks/config.go -pkg mocks -skip-ensure -fmt goimports . Config
//go:generate moq -out mocks/runner.go -pkg mocks -skip-ensure -fmt goimports . Runner

// Rest implement http api invoking remote execution for requested tasks
type Rest struct {
	Listen      string
	Version     string
	SecretKey   string
	Config      Config
	Runner      Runner
	UpdateDelay time.Duration
	DataStore store.Store
}

// Config declares command loader from config for given tasks
type Config interface {
	GetTaskCommand(name string) (command string, ok bool)
}

// Runner executes commands
type Runner interface {
	Run(ctx context.Context, command string, logWriter io.Writer, uuid string) error
}

// Run starts http server and closes on context cancellation
func (s *Rest) Run(ctx context.Context) error {
	log.Printf("[INFO] start http server on %s", s.Listen)

	httpServer := &http.Server{
		Addr:              s.Listen,
		Handler:           s.router(),
		ReadHeaderTimeout: time.Second,
		WriteTimeout:      30 * time.Second,
		IdleTimeout:       time.Second,
		ErrorLog:          log.ToStdLogger(log.Default(), "WARN"),
	}

	go func() {
		<-ctx.Done()
		if httpServer != nil {
			if err := httpServer.Close(); err != nil {
				log.Printf("[ERROR] failed to close http server, %v", err)
			}
		}

	}()

	return httpServer.ListenAndServe()
}

func (s *Rest) router() http.Handler {
	router := chi.NewRouter()
	router.Use(rest.Recoverer(log.Default()))
	router.Use(rest.Throttle(100)) // limit total number of the running requests
	router.Use(rest.AppInfo("updater", "jtrw", s.Version))
	router.Use(rest.Ping)
	router.Use(tollbooth_chi.LimitHandler(tollbooth.NewLimiter(10, nil)))
	if s.UpdateDelay > 0 {
		router.Use(s.slowMiddleware)
	}

	router.Get("/update/{task}/{key}", s.taskCtrl)
	router.Post("/update", s.taskPostCtrl)
	router.Get("/info/{uuid}", s.taskInfo)
	return router
}

func (s *Rest) taskInfo(w http.ResponseWriter, r *http.Request) {
    uuid := chi.URLParam(r, "uuid")
    str := s.DataStore.Get(store.BUCKET_KEY, uuid)
    if len(string(str)) <= 0 {
         fmt.Fprint(w, "Result Not Found")
         return
    }
    res := task.CommandBatchInfo{}
    json.Unmarshal([]byte(str), &res)
    fmt.Fprint(w, "Uuid: "+uuid+"\n")
    for _, item := range res.Items {
         fmt.Fprint(w, "\n--------------------------\n\n")
         fmt.Fprint(w, "> "+item.Command+"\n")
         fmt.Fprint(w, item.Result+"\n")
    }
}

// GET /update/{task}/{key}?async=[0|1]
func (s *Rest) taskCtrl(w http.ResponseWriter, r *http.Request) {
	taskName := chi.URLParam(r, "task")
	key := chi.URLParam(r, "key")
	isAsync := r.URL.Query().Get("async") == "1" || r.URL.Query().Get("async") == "yes"
	isSave := r.URL.Query().Get("save") == "1" || r.URL.Query().Get("save") == "yes"
	s.execTask(w, r, key, taskName, isAsync, isSave)
}

// POST /update
func (s *Rest) taskPostCtrl(w http.ResponseWriter, r *http.Request) {
	req := struct {
		Task   string `json:"task"`
		Secret string `json:"secret"`
		Async  bool   `json:"async"`
		Save   bool   `json:"save"`
	}{}

	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "failed to decode request", http.StatusBadRequest)
		return
	}
	s.execTask(w, r, req.Secret, req.Task, req.Async, req.Save)
}

func (s *Rest) execTask(w http.ResponseWriter, r *http.Request, secret, taskName string, isAsync bool, isSave bool) {
	if subtle.ConstantTimeCompare([]byte(secret), []byte(s.SecretKey)) != 1 {
		http.Error(w, "rejected", http.StatusForbidden)
		return
	}

	command, ok := s.Config.GetTaskCommand(taskName)
	if !ok {
		http.Error(w, "unknown command", http.StatusBadRequest)
		return
	}

	log.Printf("[INFO] invoke task %s", taskName)

    uuidStr := ""
    if isSave {
        uuidStr = uuid.New().String()
    }

	if isAsync {
		go func() {
			if err := s.Runner.Run(context.Background(), command, log.ToWriter(log.Default(), ">"), uuidStr); err != nil {
				log.Printf("[WARN] failed command")
				return
			}
		}()
		if isSave {
		    rest.RenderJSON(w, rest.JSON{"submitted": "ok", "task": taskName, "uuid": uuidStr})
		    return
		}
		rest.RenderJSON(w, rest.JSON{"submitted": "ok", "task": taskName})
		return
	}

	if err := s.Runner.Run(r.Context(), command, log.ToWriter(log.Default(), ">"), uuidStr); err != nil {
		http.Error(w, "failed command", http.StatusInternalServerError)
		return
	}

    if isSave {
        rest.RenderJSON(w, rest.JSON{"updated": "ok", "task": taskName, "uuid": uuidStr})
        return
    }
    rest.RenderJSON(w, rest.JSON{"updated": "ok", "task": taskName})
}

// middleware for slowing requests downs
func (s *Rest) slowMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(s.UpdateDelay)
		next.ServeHTTP(w, r)
	})
}
